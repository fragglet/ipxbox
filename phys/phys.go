// Package phys implements an interface for reading/writing IPX packets
// to a physical network interface.
package phys

import (
	"io"
	"net"
	"sync"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

var (
	_ = (io.ReadWriteCloser)(&Phys{})
)

// DuplexEthernetStream extends gopacket.PacketDataSource to an interface
// where packets can be both read and written.
type DuplexEthernetStream interface {
	gopacket.PacketDataSource

	Close()
	WritePacketData([]byte) error
}

// Phys implements the Reader and Writer interfaces to allow IPX packets to
// be read from and written to a physical network interface.
type Phys struct {
	stream DuplexEthernetStream
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

func NewPhys(stream DuplexEthernetStream, framer Framer) *Phys {
	return &Phys{
		stream: stream,
		ps:     gopacket.NewPacketSource(stream, layers.LinkTypeEthernet),
		framer: framer,
	}
}

// copyLoop reads packets from a and writes them to b.
func copyLoop(a, b DuplexEthernetStream) error {
	for {
		frame, _, err := a.ReadPacketData()
		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		}
		if err := b.WritePacketData(frame); err != nil {
			return err
		}
	}
}

// CopyFrames starts a background process that copies packets between the
// given two streams.
func CopyFrames(a, b DuplexEthernetStream) error {
	var wg sync.WaitGroup
	wg.Add(2)
	var err1, err2 error
	go func() {
		err1 = copyLoop(a, b)
		wg.Done()
	}()
	go func() {
		err2 = copyLoop(b, a)
		wg.Done()
	}()
	wg.Wait()
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}
