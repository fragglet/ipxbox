package pptp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

const (
	greProtocol   = 47
	recvQueueSize = 4
)

var (
	wrongLayers         = errors.New("layers not as expected: want IP->GRE")
	wrongGREFields      = errors.New("GRE fields wrong: want version=1, ethernet type PPP")
	unknownSession      = errors.New("packet for an unknown GRE session")
	outOfSequencePacket = errors.New("out of sequence packet received")
	recvQueueOverflow   = errors.New("session receive queue is full")
)

var _ = (io.ReadWriteCloser)(&greSession{})

// greSession is used to send and receive packets for a particular PPP-over-GRE
// session.
type greSession struct {
	s                           *greServer
	closed                      bool
	recvQueue                   chan gopacket.Packet
	addr                        net.IP
	sendCallID, recvCallID      uint16
	sentSeq, recvSeq, recvAcked uint32
}

func (s *greSession) recvPacket(p []byte) (int, error) {
	pkt, ok := <-s.recvQueue
	if !ok {
		return 0, io.EOF
	}
	ls := pkt.Layers()
	greHeader := ls[1].(*layers.GRE)
	// RFC 2637 mandates that "out of sequence packets between the PNS and
	// PAC MUST be silently discarded [or reordered]" because PPP cannot
	// handle out-of-order packets.
	// TODO: Consider selectively breaking this requirement for some
	// packets, ie. encapsulated IPX frames.
	if greHeader.SeqPresent {
		if greHeader.Seq < s.recvSeq {
			return 0, outOfSequencePacket
		}
		// TODO: if we don't otherwise send a packet, send an empty ack packet
		s.recvSeq = greHeader.Seq
	}
	result := ls[1].LayerPayload()
	copy(p[0:len(result)], result)
	return len(result), nil
}

func (s *greSession) Read(p []byte) (int, error) {
	for {
		cnt, err := s.recvPacket(p)
		switch err {
		case nil:
			// We might have successfully received a packet, but if
			// it was just an ack it might have been zero length,
			// so try again.
			if cnt > 0 {
				return cnt, nil
			}
		case outOfSequencePacket:
			// try again
		default:
			return 0, err
		}
	}
}

func (s *greSession) sendPacket(frame []byte) (int, error) {
	greHeader := &layers.GRE{
		Protocol:   layers.EthernetTypePPP,
		KeyPresent: true,
		Key:        uint32(len(frame)<<16) | uint32(s.sendCallID),
		Version:    1, // Enhanced GRE
	}
	if len(frame) > 0 {
		greHeader.Seq = s.sentSeq
		greHeader.SeqPresent = true
		s.sentSeq++
	}
	if s.recvAcked < s.recvSeq {
		greHeader.Ack = s.recvSeq
		greHeader.AckPresent = true
		s.recvAcked = s.recvSeq
	}
	buf := gopacket.NewSerializeBuffer()
	var opts gopacket.SerializeOptions
	gopacket.SerializeLayers(buf, opts,
		greHeader,
		gopacket.Payload(frame),
	)
	return s.s.conn.WriteToIP(buf.Bytes(), &net.IPAddr{
		IP: s.addr,
	})
}

func (s *greSession) Write(frame []byte) (int, error) {
	return s.sendPacket(frame)
}

func (s *greSession) Close() error {
	sk := s.sessionKey()
	s.s.mu.Lock()
	defer s.s.mu.Unlock()
	if !s.closed {
		delete(s.s.sessions, *sk)
		close(s.recvQueue)
		s.closed = true
	}
	return nil
}

func (s *greSession) sessionKey() *sessionKey {
	return &sessionKey{
		IP:     s.addr.String(),
		CallID: s.recvCallID,
	}
}

type sessionKey struct {
	IP     string
	CallID uint16
}

type greServer struct {
	conn     *net.IPConn
	sessions map[sessionKey]*greSession
	mu       sync.Mutex
}

func startGREServer() (*greServer, error) {
	conn, err := net.ListenIP(fmt.Sprintf("ip4:%d", greProtocol), nil)
	if err != nil {
		return nil, err
	}
	return &greServer{
		conn:     conn,
		sessions: make(map[sessionKey]*greSession),
	}, nil
}

func (s *greServer) startSession(remoteAddr net.IP, sendCallID, recvCallID uint16) (*greSession, error) {
	session := &greSession{
		s:          s,
		addr:       remoteAddr,
		recvQueue:  make(chan gopacket.Packet, recvQueueSize),
		sendCallID: sendCallID,
		recvCallID: recvCallID,
	}
	sk := session.sessionKey()
	s.mu.Lock()
	s.sessions[*sk] = session
	s.mu.Unlock()
	return session, nil
}

func (s *greServer) processPacket(pkt gopacket.Packet) error {
	ls := pkt.Layers()
	if len(ls) < 2 || ls[0].LayerType() != layers.LayerTypeIPv4 || ls[1].LayerType() != layers.LayerTypeGRE {
		return wrongLayers
	}
	ipHeader := ls[0].(*layers.IPv4)
	greHeader := ls[1].(*layers.GRE)
	if greHeader.Version != 1 || greHeader.Protocol != layers.EthernetTypePPP {
		return wrongGREFields
	}
	// In PPTP modified GRE, the bottom two octets of the key field are
	// used to contain the call ID.
	if !greHeader.KeyPresent {
		return wrongGREFields
	}
	sk := &sessionKey{
		IP:     ipHeader.SrcIP.String(),
		CallID: uint16(greHeader.Key & 0xffff),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[*sk]
	if !ok || session.closed {
		return unknownSession
	}
	// Try to place onto session's receive queue, but don't block.
	select {
	case session.recvQueue <- pkt:
		return nil
	default:
		return recvQueueOverflow
	}
}

func (s *greServer) Run() error {
	var recvBuf [1500]byte
	for {
		cnt, err := s.conn.Read(recvBuf[:])
		if err != nil {
			return err
		}
		pkt := gopacket.NewPacket(recvBuf[:cnt], layers.LayerTypeIPv4, gopacket.Default)
		// TODO: Log errors returned by processPacket?
		s.processPacket(pkt)
	}
}

func (s *greServer) Close() error {
	s.mu.Lock()
	for _, session := range s.sessions {
		close(session.recvQueue)
		session.closed = true
	}
	s.mu.Unlock()
	return s.conn.Close()
}
