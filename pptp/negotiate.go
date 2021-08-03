package pptp

import (
	"bytes"
	"github.com/fragglet/ipxbox/pptp/lcp"

	"github.com/google/gopacket"
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

type negotiator struct {
	localOptions                  map[lcp.OptionType]*option
	remoteOptions                 map[lcp.OptionType]*option
	localComplete, remoteComplete bool
}

func (n *negotiator) getLCP(pkt gopacket.Packet) *lcp.LCP {
	l := pkt.Layer(lcp.LayerTypeLCP)
	if l == nil {
		return nil
	}
	return l.(*lcp.LCP)
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
		// TODO: send Configure-Reject
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
		// TODO: send Configure-Nak
	}
	// Update with all new values.
	for ot, value := range newValues {
		n.remoteOptions[ot].value = value
	}
	// TODO: send Configure-Ack
	n.remoteComplete = true
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
		// Fail with error
		return
	}
	if len(rejectedOpts) > 0 {
		// Fail with error
		return
	}

	// Update the new values.
	for ot, value := range values {
		o := n.localOptions[ot]
		o.value = value
	}
	// TODO: Send a new Configure-Request.
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

func (n *negotiator) recvPacket(pkt gopacket.Packet) {
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

// TODO: periodically send Configure-Request if no response
// TODO: state machine to track whether we have succeeded, failed, error, etc.
