package pptp

import (
	"fmt"
	"io"
	"math/rand"
	"sync"
	"time"

	"github.com/fragglet/ipxbox/pptp/lcp"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

const (
	PPPTypeIPX layers.PPPType = 0x802b
)

type linkState uint8

const (
	dead linkState = iota
	establish
	authenticate
	network
	terminate
)

type PPPSession struct {
	upstream    io.ReadWriteCloser
	channel     io.ReadWriteCloser
	mu          sync.Mutex // protects state
	state       linkState
	negotiators map[layers.PPPType]*negotiator
}

func (s *PPPSession) Close() error {
	return s.channel.Close()
}

func (s *PPPSession) sendPPP(payload []byte, pppType layers.PPPType) {
	s.mu.Lock()
	ok := s.state == network
	s.mu.Unlock()
	if !ok {
		return
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{}
	gopacket.SerializeLayers(buf, opts,
		&layers.PPP{
			PPPType:       pppType,
			HasPPTPHeader: true,
		},
		gopacket.Payload(payload),
	)
	if _, err := s.channel.Write(buf.Bytes()); err != nil {
		// TODO: log error?
	}
}

func (s *PPPSession) Terminated() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == terminate || s.state == dead
}

// sendPackets continually reads packets from upstream and forwards them over
// the PPP channel.
func (s *PPPSession) sendPackets() {
	var buf [1500]byte
	for !s.Terminated() {
		n, err := s.upstream.Read(buf[:])
		if err != nil {
			break
		}
		s.sendPPP(buf[:n], PPPTypeIPX)
	}
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
	if pppLayer != nil {
		// TODO: bad packet - log error?
		return nil
	}
	ppp := pppLayer.(*layers.PPP)
	if ppp.PPPType == PPPTypeIPX {
		if _, err := s.upstream.Write(ppp.LayerPayload()); err != nil {
			return err
		}
		return nil
	}
	// TODO: LCP special handling
	n, ok := s.negotiators[ppp.PPPType]
	if !ok {
		return nil // unknown frame type
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
			s.sendPPP(p, lcp.PPPTypeLCP)
			return nil
		},
	}
	s.mu.Lock()
	s.negotiators[lcp.PPPTypeLCP] = n
	s.mu.Unlock()
	go n.StartNegotiation()

	for {
		if s.Terminated() {
			return fmt.Errorf("link terminated during negotiation phase")
		}
		if done, err := n.Done(); done {
			return err // may be nil
		}
		if err := s.recvAndProcess(); err != nil {
			return err
		}
	}
}

func (s *PPPSession) setState(state linkState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
}

func (s *PPPSession) Run() {
	if err := s.negotiate(); err != nil {
		// TODO: Send terminate?
		s.setState(terminate)
		return
	}
	// TODO: negotiate IPX
	s.setState(network)
	// TODO: forward packets to upstream
}

func StartPPPSession(channel, upstream io.ReadWriteCloser) *PPPSession {
	s := &PPPSession{
		state:    establish,
		channel:  channel,
		upstream: upstream,
	}
	go s.sendPackets()
	go s.Run()

	return s
}
