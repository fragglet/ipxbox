package ppp

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fragglet/ipxbox/ppp/lcp"
	"github.com/google/gopacket"
)

const (
	maxConfigureRequests = 5
	requestTimeout       = 1 * time.Second
)

type option struct {
	value    []byte
	validate func(o *option, newValue []byte) bool
}

func (o *option) validateValue(value []byte) bool {
	if o.validate == nil {
		return true
	}
	// Exactly the same thing we already have?
	if (o.value == nil) == (value == nil) && bytes.Equal(o.value, value) {
		return true
	}
	return o.validate(o, value)
}

// nonNegotiable is a validator function that rejects any change in value
// whatsoever. Only the value we have provided is acceptable.
func nonNegotiable(o *option, newValue []byte) bool {
	return false
}

// requiredOption is a validator function that will accept any change except
// if the new value is nil - ie. a value must be provided for this option.
func requiredOption(o *option, newValue []byte) bool {
	return newValue != nil
}

type negotiator struct {
	localOptions, remoteOptions   map[lcp.OptionType]*option
	sendPPP                       func(p []byte) error
	localComplete, remoteComplete bool
	requestSequence               uint8
	requestSendTime               time.Time
	mu                            sync.Mutex
	err                           error
}

func (n *negotiator) getLCP(pkt gopacket.Packet) *lcp.LCP {
	l := pkt.Layer(lcp.LayerTypeLCP)
	if l == nil {
		return nil
	}
	return l.(*lcp.LCP)
}

func (n *negotiator) sendPacket(l *lcp.LCP) {
	payload, err := l.MarshalBinary()
	if err != nil {
		return
	}
	if err := n.sendPPP(payload); err != nil {
		n.err = err
		return
	}
}

func (n *negotiator) handleConfigureRequest(l *lcp.LCP) {
	cd := l.Data.(*lcp.ConfigureData)
	unknownOpts := []lcp.Option{}
	for _, opt := range cd.Options {
		if _, ok := n.remoteOptions[opt.Type]; !ok {
			unknownOpts = append(unknownOpts, opt)
		}
	}
	if len(unknownOpts) > 0 {
		// Some options are not recognized (not in remoteOptions).
		n.sendPacket(&lcp.LCP{
			Type:       lcp.ConfigureReject,
			Identifier: l.Identifier,
			Data: &lcp.ConfigureData{
				Options: unknownOpts,
			},
		})
		return
	}
	// Build up a complete map of all new values. Some may be missing.
	newValues := make(map[lcp.OptionType][]byte)
	for ot := range n.remoteOptions {
		newValues[ot] = nil
	}
	for _, opt := range cd.Options {
		newValues[opt.Type] = append([]byte{}, opt.Data...)
	}
	// Validate all new values.
	badOpts := map[lcp.OptionType]bool{}
	for ot, value := range newValues {
		if !n.remoteOptions[ot].validateValue(value) {
			badOpts[ot] = true
		}
	}
	if len(badOpts) > 0 {
		// Some options have been rejected by validators.
		// We send back a list of them, with our suggested value.
		replyOpts := []lcp.Option{}
		for _, opt := range cd.Options {
			if badOpts[opt.Type] {
				replyOpts = append(replyOpts, lcp.Option{
					Type: opt.Type,
					Data: n.remoteOptions[opt.Type].value,
				})
			}
		}
		// Some options were required and missing?
		for ot := range badOpts {
			if newValues[ot] == nil {
				replyOpts = append(replyOpts, lcp.Option{
					Type: ot,
					Data: n.remoteOptions[ot].value,
				})
			}
		}
		n.sendPacket(&lcp.LCP{
			Type:       lcp.ConfigureNak,
			Identifier: l.Identifier,
			Data:       &lcp.ConfigureData{Options: replyOpts},
		})
		return
	}
	// Update with all new values.
	for ot, value := range newValues {
		n.remoteOptions[ot].value = value
	}
	n.remoteComplete = true

	// Send ack with options that exactly match the request, as per RFC.
	n.sendPacket(&lcp.LCP{
		Type:       lcp.ConfigureAck,
		Identifier: l.Identifier,
		Data:       l.Data,
	})
}

func (n *negotiator) sendConfigureRequest() {
	opts := []lcp.Option{}
	for ot, opt := range n.localOptions {
		if opt.value != nil {
			opts = append(opts, lcp.Option{
				Type: ot,
				Data: opt.value,
			})
		}
	}
	n.sendPacket(&lcp.LCP{
		Type:       lcp.ConfigureRequest,
		Identifier: n.requestSequence,
		Data:       &lcp.ConfigureData{Options: opts},
	})
	n.requestSequence++
	n.requestSendTime = time.Now()
}

// applyNewValues sets new values in localOptions, but first performs
// validation that the new values are acceptable to us.
func (n *negotiator) applyNewValues(values map[lcp.OptionType][]byte) {
	unknownOpts := []lcp.OptionType{}
	rejectedOpts := []lcp.OptionType{}
	for ot, value := range values {
		o, ok := n.localOptions[ot]
		if !ok {
			unknownOpts = append(unknownOpts, ot)
			continue
		}
		if !o.validateValue(value) {
			rejectedOpts = append(rejectedOpts, ot)
		}
	}
	if len(unknownOpts) > 0 {
		n.err = fmt.Errorf("peer requested unrecognized options: %+v", unknownOpts)
		return
	}
	if len(rejectedOpts) > 0 {
		optDescs := []string{}
		for _, ot := range rejectedOpts {
			desc := fmt.Sprintf("%+v for option %d", values[ot], ot)
			if values[ot] == nil {
				desc = fmt.Sprintf("rejected required option %d", ot)
			}
			optDescs = append(optDescs, desc)
		}
		n.err = fmt.Errorf("peer wanted unacceptable option values: %+v", strings.Join(optDescs, ";"))
		return
	}

	// Update the new values.
	for ot, value := range values {
		o := n.localOptions[ot]
		o.value = value
	}
	// Send a new Configure-Request with our updated values.
	n.sendConfigureRequest()
}

func (n *negotiator) handleConfigureReject(l *lcp.LCP) {
	cd := l.Data.(*lcp.ConfigureData)

	// A Configure-Reject is saying "don't send these again". We do this
	// by setting values to nil.
	values := make(map[lcp.OptionType][]byte)
	for _, opt := range cd.Options {
		values[opt.Type] = nil
	}

	n.applyNewValues(values)
}

func (n *negotiator) handleConfigureNak(l *lcp.LCP) {
	cd := l.Data.(*lcp.ConfigureData)

	values := make(map[lcp.OptionType][]byte)
	for _, opt := range cd.Options {
		values[opt.Type] = append([]byte{}, opt.Data...)
	}

	n.applyNewValues(values)
}

func (n *negotiator) RecvPacket(pkt gopacket.Packet) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.err != nil {
		// Once an error occurs, we stop any more packet processing.
		return
	}
	l := n.getLCP(pkt)
	if l == nil {
		return
	}
	switch l.Type {
	case lcp.ConfigureRequest:
		n.handleConfigureRequest(l)
	case lcp.ConfigureAck:
		n.localComplete = true
	case lcp.ConfigureNak:
		n.handleConfigureNak(l)
	case lcp.ConfigureReject:
		n.handleConfigureReject(l)
	}
}

// maybeSendRequest sends a new Configure-Request if it has been too long
// since the last one was received.
func (n *negotiator) maybeSendRequest() {
	now := time.Now()
	if now.Before(n.requestSendTime.Add(requestTimeout)) {
		return
	}
	if n.requestSequence >= maxConfigureRequests+1 {
		n.err = fmt.Errorf("failed to negotiate after sending %d Configure-Requests", maxConfigureRequests)
		return
	}
	n.sendConfigureRequest()
}

func (n *negotiator) StartNegotiation() {
	n.requestSequence = 1
	n.err = nil
	for {
		n.mu.Lock()
		done := n.localComplete || n.err != nil
		if !done {
			n.maybeSendRequest()
		}
		n.mu.Unlock()
		if done {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (n *negotiator) Done() (bool, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	done := n.localComplete && n.remoteComplete || n.err != nil
	return done, n.err
}
