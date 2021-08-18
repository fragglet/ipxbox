package ppp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/ppp/lcp"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

const (
	PPPTypeIPX layers.PPPType = 0x002b
)

var (
	// supportedProtocols defines all the PPP protocol types that we
	// support. Any other type triggers a Protocol-Reject response.
	supportedProtocols = map[layers.PPPType]bool{
		PPPTypeIPX:       true,
		lcp.PPPTypeIPXCP: true,
		lcp.PPPTypeLCP:   true,
	}
)

type linkState uint8

const (
	stateDead linkState = iota
	stateEstablish
	stateAuthenticate
	stateNetwork
	stateTerminate
)

type Session struct {
	node               network.Node
	channel            io.ReadWriteCloser
	mu                 sync.Mutex // protects state
	state              linkState
	negotiators        map[layers.PPPType]*negotiator
	numProtocolRejects uint8
	magicNumber        uint32
	terminateError     error
}

func (s *Session) Close() error {
	s.node.Close()
	return s.channel.Close()
}

func (s *Session) sendPPP(payload []byte, pppType layers.PPPType) error {
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{}
	gopacket.SerializeLayers(buf, opts,
		&layers.PPP{
			PPPType:       pppType,
			HasPPTPHeader: true,
		},
		gopacket.Payload(payload),
	)
	_, err := s.channel.Write(buf.Bytes())
	return err
}

func (s *Session) sendLCP(l *lcp.LCP) error {
	payload, err := l.MarshalBinary()
	if err != nil {
		return err
	}
	return s.sendPPP(payload, lcp.PPPTypeLCP)
}

func (s *Session) Terminated() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == stateTerminate || s.state == stateDead
}

// sendPackets continually reads packets from upstream and forwards them over
// the PPP channel.
func (s *Session) sendPackets() {
	for !s.Terminated() {
		packet, err := s.node.ReadPacket()
		if err != nil {
			break
		}
		s.mu.Lock()
		ok := s.state == stateNetwork
		s.mu.Unlock()
		if !ok {
			// Not yet in network state
			continue
		}
		if err := s.sendPPP(packet.Payload, PPPTypeIPX); err != nil {
			break
		}
	}
}

func (s *Session) handleLCP(l *lcp.LCP) bool {
	switch l.Type {
	case lcp.TerminateRequest:
		// Send ack and then immediately shut down.
		s.sendLCP(&lcp.LCP{
			Type:       lcp.TerminateAck,
			Identifier: l.Identifier,
		})
		s.Close()
	case lcp.ProtocolReject:
		// All the protocols we support are mandatory for the client to
		// support. More specifically if they don't support IPX they
		// won't be able to do anything useful here.
		prd := l.Data.(*lcp.ProtocolRejectData)
		err := fmt.Errorf("protocol %v must be supported to use this server", prd.PPPType)
		s.Terminate(err)
	case lcp.EchoRequest:
		s.sendLCP(&lcp.LCP{
			Type:       lcp.EchoReply,
			Identifier: l.Identifier,
			Data: &lcp.EchoData{
				MagicNumber: s.magicNumber,
			},
		})
	default:
		return false
	}
	return true
}

// recvAndProcess waits until a PPP frame is received and processes it.
func (s *Session) recvAndProcess() error {
	var buf [1500]byte
	// TODO: Send Echo-Requests when link idle, and time out eventually
	nbytes, err := s.channel.Read(buf[:])
	if err != nil {
		return err
	}
	pkt := gopacket.NewPacket(buf[:nbytes], layers.LayerTypePPP, gopacket.Default)
	pppLayer := pkt.Layer(layers.LayerTypePPP)
	if pppLayer == nil {
		// TODO: bad packet - log error?
		return nil
	}
	ppp := pppLayer.(*layers.PPP)
	if !supportedProtocols[ppp.PPPType] {
		s.sendLCP(&lcp.LCP{
			Type:       lcp.ProtocolReject,
			Identifier: s.numProtocolRejects,
			Data: &lcp.ProtocolRejectData{
				PPPType: ppp.PPPType,
				Data:    ppp.LayerPayload(),
			},
		})
		s.numProtocolRejects++
		return nil
	}

	if ppp.PPPType == PPPTypeIPX {
		packet := &ipx.Packet{}
		if err := packet.UnmarshalBinary(ppp.LayerPayload()); err != nil {
			// TODO: Bad packet - log error?
			return nil
		}
		s.node.WritePacket(packet)
		// Don't return error; it may have just been a filtered
		// packet.
		return nil
	}
	if ppp.PPPType == lcp.PPPTypeLCP {
		l := pkt.Layer(lcp.LayerTypeLCP)
		if l == nil {
			return nil
		}
		if s.handleLCP(l.(*lcp.LCP)) {
			return nil
		}
	}
	if n, ok := s.negotiators[ppp.PPPType]; ok {
		n.RecvPacket(pkt)
	}
	return nil
}

// negotiate runs the basic LCP negotiation phase of PPP link setup.
func (s *Session) negotiate() error {
	magicNumber := []byte{0, 0, 0, 0}
	rand.Seed(time.Now().Unix())
	rand.Read(magicNumber)
	localOptions := map[lcp.OptionType]*option{
		lcp.OptionMagicNumber: &option{
			value:    magicNumber,
			validate: nonNegotiable,
		},
	}
	remoteOptions := map[lcp.OptionType]*option{
		lcp.OptionMagicNumber: &option{
			value:    []byte{0, 0, 0, 0},
			validate: requiredOption,
		},
	}

	n := &negotiator{
		localOptions:  localOptions,
		remoteOptions: remoteOptions,
		sendPPP: func(p []byte) error {
			return s.sendPPP(p, lcp.PPPTypeLCP)
		},
	}
	s.negotiators[lcp.PPPTypeLCP] = n
	go n.StartNegotiation()

	for {
		if s.Terminated() {
			return fmt.Errorf("link terminated during negotiation phase")
		}
		if done, err := n.Done(); done {
			if err != nil {
				return err
			}
			break
		}
		if err := s.recvAndProcess(); err != nil {
			return err
		}
	}
	// Negotiation successful
	s.magicNumber = binary.BigEndian.Uint32(magicNumber)
	return nil
}

// negotiateIPX runs IPXCP negotiation phase of PPP link setup.
func (s *Session) negotiateIPX() error {
	localOptions := map[lcp.OptionType]*option{
		lcp.OptionIPXNetwork: &option{
			value: []byte{0, 0, 0, 0},
		},
		lcp.OptionIPXNode: &option{
			value: []byte{0, 0, 0, 0, 0, 0},
		},
	}
	// TODO: Make address negotiable so that client can supply a
	// desired address?
	addr := s.node.Address()
	remoteOptions := map[lcp.OptionType]*option{
		lcp.OptionIPXNode: &option{
			value:    addr[:],
			validate: nonNegotiable,
		},
	}

	n := &negotiator{
		localOptions:  localOptions,
		remoteOptions: remoteOptions,
		sendPPP: func(p []byte) error {
			return s.sendPPP(p, lcp.PPPTypeIPXCP)
		},
	}
	s.negotiators[lcp.PPPTypeIPXCP] = n
	go n.StartNegotiation()

	for {
		if s.Terminated() {
			return fmt.Errorf("link terminated during IPX protocol negotiation")
		}
		if done, err := n.Done(); done {
			return err // may be nil
		}
		if err := s.recvAndProcess(); err != nil {
			return err
		}
	}
}

func (s *Session) runNetwork() error {
	s.setState(stateNetwork)
	for !s.Terminated() {
		if err := s.recvAndProcess(); err != nil {
			return err
		}
	}
	return nil
}

// setState changes the session state; false is returned if it is already
// in that state.
func (s *Session) setState(state linkState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == state {
		return false
	}
	s.state = state
	return true
}

// Terminate initiates the link shutdown process by sending a Terminate-Request
// to the client and then calling Close().
func (s *Session) Terminate(err error) {
	if !s.setState(stateTerminate) {
		return
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	s.sendLCP(&lcp.LCP{
		Type: lcp.TerminateRequest,
		Data: &lcp.TerminateData{
			Data: []byte(msg),
		},
	})
	s.Close()
	s.terminateError = err
}

func (s *Session) doRun() error {
	if err := s.negotiate(); err != nil {
		return err
	}
	if err := s.negotiateIPX(); err != nil {
		return err
	}
	if err := s.runNetwork(); err != nil {
		return err
	}
	return nil
}

// Run implements the main goroutine that establishes the PPP connection, does
// negotiation and then runs the main loop that receives PPP frames and
// forwards the encapsulated IPX frames upstream. When it returns, the session
// has terminated (either normally or due to failure to negotiate).
func (s *Session) Run() error {
	go s.sendPackets()
	err := s.doRun()
	// If the error is because the connection was closed or the node was
	// shut down, ignore it. This is a normal part of shutdown process.
	// TODO: Use net.ErrClosed; it is too new at time of writing
	if errors.Is(err, io.ErrClosedPipe) || err != nil && strings.Contains(err.Error(), "closed") {
		err = nil
	}
	if s.terminateError != nil {
		err = s.terminateError
	} else {
		s.Terminate(err)
	}
	return err
}

func NewSession(channel io.ReadWriteCloser, node network.Node) *Session {
	return &Session{
		state:       stateEstablish,
		channel:     channel,
		node:        node,
		negotiators: make(map[layers.PPPType]*negotiator),
	}
}
