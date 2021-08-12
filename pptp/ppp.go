package pptp

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"time"

	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/pptp/lcp"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

const (
	PPPTypeIPX layers.PPPType = 0x002b
)

type linkState uint8

const (
	stateDead linkState = iota
	stateEstablish
	stateAuthenticate
	stateNetwork
	stateTerminate
)

type PPPSession struct {
	node               network.Node
	channel            io.ReadWriteCloser
	mu                 sync.Mutex // protects state
	state              linkState
	negotiators        map[layers.PPPType]*negotiator
	numProtocolRejects uint8
	magicNumber        uint32
}

func (s *PPPSession) Close() error {
	return s.channel.Close()
}

func (s *PPPSession) sendPPP(payload []byte, pppType layers.PPPType) error {
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

func (s *PPPSession) sendLCP(l *lcp.LCP) error {
	payload, err := l.MarshalBinary()
	if err != nil {
		return err
	}
	return s.sendPPP(payload, lcp.PPPTypeLCP)
}

func (s *PPPSession) Terminated() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == stateTerminate || s.state == stateDead
}

// sendPackets continually reads packets from upstream and forwards them over
// the PPP channel.
func (s *PPPSession) sendPackets() {
	var buf [1500]byte
	for !s.Terminated() {
		n, err := s.node.Read(buf[:])
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
		if err := s.sendPPP(buf[:n], PPPTypeIPX); err != nil {
			break
		}
	}
}

func (s *PPPSession) handleLCP(l *lcp.LCP) bool {
	switch l.Type {
	case lcp.TerminateRequest:
		// TODO
	case lcp.ProtocolReject:
		// TODO: Send terminate
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
func (s *PPPSession) recvAndProcess() error {
	var buf [1500]byte
	// TODO: Timeout
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
	if ppp.PPPType == PPPTypeIPX {
		_, err := s.node.Write(ppp.LayerPayload())
		return err
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
	n, ok := s.negotiators[ppp.PPPType]
	if !ok {
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
	n.RecvPacket(pkt)
	return nil
}

// negotiate runs the basic LCP negotiation phase of PPP link setup.
func (s *PPPSession) negotiate() error {
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
func (s *PPPSession) negotiateIPX() error {
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

func (s *PPPSession) runNetwork() error {
	s.setState(stateNetwork)
	for !s.Terminated() {
		if err := s.recvAndProcess(); err != nil {
			return err
		}
	}
	return nil
}

func (s *PPPSession) setState(state linkState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
}

// run implements the main goroutine that establishes the PPP connection, does
// negotiation and then runs the main loop that receives PPP frames and
// forwards the encapsulated IPX frames upstream.
func (s *PPPSession) run() {
	if err := s.negotiate(); err != nil {
		// TODO: Send terminate?
		s.setState(stateTerminate)
		return
	}
	if err := s.negotiateIPX(); err != nil {
		// TODO: Send terminate?
		s.setState(stateTerminate)
		return
	}
	if err := s.runNetwork(); err != nil {
		return
	}
}

func StartPPPSession(channel io.ReadWriteCloser, node network.Node) *PPPSession {
	s := &PPPSession{
		state:       stateEstablish,
		channel:     channel,
		node:        node,
		negotiators: make(map[layers.PPPType]*negotiator),
	}
	go s.sendPackets()
	go s.run()

	return s
}
