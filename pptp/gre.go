package pptp

import (
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

const (
	greProtocol = 47
)

var (
	wrongLayers         = errors.New("layers not as expected: want IP->GRE")
	wrongGREFields      = errors.New("GRE fields wrong: want version=1, ethernet type PPP")
	wrongSession        = errors.New("packet for a different GRE session")
	outOfSequencePacket = errors.New("out of sequence packet received")
)

var _ = (io.ReadWriteCloser)(&greSession{})

// greSession is used to send and receive packets for a particular PPP-over-GRE
// session.
type greSession struct {
	conn                        net.Conn
	sendCallID, recvCallID      uint32
	sentSeq, recvSeq, recvAcked uint32
}

func (s *greSession) recvPacket(p []byte) (int, error) {
	cnt, err := s.conn.Read(p)
	if err != nil {
		return 0, err
	}
	pkt := gopacket.NewPacket(p[:cnt], layers.LayerTypeIPv4, gopacket.NoCopy)
	ls := pkt.Layers()
	if len(ls) < 2 || ls[0].LayerType() != layers.LayerTypeIPv4 || ls[1].LayerType() != layers.LayerTypeGRE {
		return 0, wrongLayers
	}
	greHeader := ls[1].(*layers.GRE)
	if greHeader.Version != 1 || greHeader.Protocol != layers.EthernetTypePPP {
		return 0, wrongGREFields
	}
	if !greHeader.KeyPresent || greHeader.Key != s.recvCallID || !greHeader.SeqPresent {
		return 0, wrongSession
	}
	if greHeader.Seq < s.recvSeq {
		return 0, outOfSequencePacket
	}
	s.recvSeq = greHeader.Seq
	result := ls[1].LayerPayload()
	copy(p[0:len(result)], result)
	return len(result), nil
}

func (s *greSession) Read(p []byte) (int, error) {
	for {
		cnt, err := s.recvPacket(p)
		switch err {
		case nil:
			return cnt, nil
		case wrongLayers, wrongGREFields, wrongSession, outOfSequencePacket:
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
		Key:        s.sendCallID,
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
	return s.conn.Write(buf.Bytes())
}

func (s *greSession) Write(frame []byte) (int, error) {
	return s.sendPacket(frame)
}

func (s *greSession) Close() error {
	return s.conn.Close()
}

func makeGREWrapper(remoteAddr net.IP, sendCallID, recvCallID uint16) (*greSession, error) {
	conn, err := net.Dial(fmt.Sprintf("ip4:%d", greProtocol), remoteAddr.String())
	if err != nil {
		return nil, err
	}
	return &greSession{
		conn: conn,
	}, nil
}
