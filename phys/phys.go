// Package phys implements an interface for reading/writing IPX packets
// to a physical network interface.
package phys

import (
	"io"
	"net"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

var (
	_ = (io.ReadWriteCloser)(&Phys{})
)

// packetDuplexStream represents the concept of a two-way stream of packets
// where packets can be both read from and written to the stream.
type packetDuplexStream interface {
	gopacket.PacketDataSource

	Close()
	WritePacketData([]byte) error
}

type Phys struct {
	stream packetDuplexStream
	ps     *gopacket.PacketSource
	framer Framer
}

func (p *Phys) Close() error {
	p.stream.Close()
	return nil
}

// Read implements the io.Reader interface, and will block until an IPX packet
// is read from the physical interface.
func (p *Phys) Read(result []byte) (int, error) {
	for {
		pkt, err := p.ps.NextPacket()
		if err != nil {
			return 0, nil
		}
		payload, ok := GetIPXPayload(pkt)
		if ok {
			cnt := len(payload)
			if len(result) < cnt {
				cnt = len(result)
			}
			copy(result[:cnt], payload[:cnt])
			return cnt, nil
		}
	}
}

// Write implements the io.Writer interface, and will write the given IPX
// packet to the physical interface.
func (p *Phys) Write(packet []byte) (int, error) {
	var hdr ipx.Header
	if err := hdr.UnmarshalBinary(packet); err != nil {
		return 0, err
	}
	dest := net.HardwareAddr(hdr.Dest.Addr[:])
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{}
	layers, err := p.framer.Frame(dest, packet)
	if err != nil {
		return 0, err
	}
	gopacket.SerializeLayers(buf, opts, layers...)
	if err := p.stream.WritePacketData(buf.Bytes()); err != nil {
		return 0, err
	}
	return len(packet), nil
}

func newPhys(stream packetDuplexStream, framer Framer) *Phys {
	return &Phys{
		stream: stream,
		ps:     gopacket.NewPacketSource(stream, layers.LinkTypeEthernet),
		framer: framer,
	}
}
