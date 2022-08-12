// Package phys implements an interface for reading/writing IPX packets
// to a physical network interface.
package phys

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network/pipe"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
)

// loopbackDetectValue is a special magic value that the TransControl
// field is set to when IPX packets are written to a physical interface.
// We use this to detect packets if they have been looped back and
// captured again due to bug #18, and discard them.
const loopbackDetectValue = 127

var (
	_ = (ipx.WriteCloser)(&Sink{})
	_ = (ipx.ReadWriteCloser)(&Phys{})
	_ = (DuplexEthernetStream)(&nonIPX{})
	_ = (PacketDataSink)(&pcapgoSinkShim{})
	_ = (PcapgoDataSink)(&pcapgo.Writer{})
)

// PacketDataSink is the complement to gopacket.PacketDataSource: the
// WritePacketData method implemented by gopacket's pcap.Handle that allows
// packets to be written to an output.
type PacketDataSink interface {
	WritePacketData([]byte) error
	Close()
}

// PcapgoDataSink is the packet writing interface implemented by the
// gopacket/pcapgo writer types.
type PcapgoDataSink interface {
	WritePacket(ci gopacket.CaptureInfo, data []byte) error
}

// DuplexEthernetStream extends gopacket.PacketDataSource to an interface
// where packets can be both read and written.
type DuplexEthernetStream interface {
	gopacket.PacketDataSource
	PacketDataSink
}

type pcapgoSinkShim struct {
	pds PcapgoDataSink
}

func (s *pcapgoSinkShim) WritePacketData(data []byte) error {
	return s.pds.WritePacket(gopacket.CaptureInfo{
		Timestamp:     time.Now(),
		CaptureLength: len(data),
		Length:        len(data),
	}, data)
}

func (s *pcapgoSinkShim) Close() {}

// Sink is an implementation of ipx.WriteCloser that frames IPX packets and
// writes them to a physical network interface.
type Sink struct {
	pds    PacketDataSink
	framer Framer
}

// WritePacket implements the ipx.Writer interface, and will write the
// given IPX packet to the physical interface.
func (s *Sink) WritePacket(packet *ipx.Packet) error {
	dest := net.HardwareAddr(packet.Header.Dest.Addr[:])
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{}
	modifiedHeader := packet.Header
	modifiedHeader.Checksum = 0
	modifiedHeader.TransControl = loopbackDetectValue
	layers, err := s.framer.Frame(dest, &ipx.Packet{
		Header: modifiedHeader,
		Payload: packet.Payload,
	})
	if err != nil {
		return err
	}
	gopacket.SerializeLayers(buf, opts, layers...)
	return s.pds.WritePacketData(buf.Bytes())
}

func (s *Sink) Close() error {
	s.pds.Close()
	return nil
}

// NewSink returns an implementation of ipx.WriteCloser that writes packets
// to the given gopacket data sink.
func NewSink(pds PacketDataSink, framer Framer) *Sink {
	return &Sink{
		pds:    pds,
		framer: framer,
	}
}

// NewPcapgoSink returns an implementation of ipx.WriteCloser that writes
// packets to the given pcapgo Writer.
func NewPcapgoSink(pds PcapgoDataSink, framer Framer) *Sink {
	return NewSink(&pcapgoSinkShim{pds}, framer)
}

// Phys is an implementation of ipx.ReadWriteCloser that reads and writes
// IPX packets from a physical network interface.
type Phys struct {
	*Sink
	ps     *gopacket.PacketSource
	rxpipe ipx.ReadWriteCloser
	nonIPX *nonIPX
	mu     sync.Mutex
}

func (p *Phys) Close() error {
	p.rxpipe.Close()
	p.mu.Lock()
	if p.nonIPX != nil {
		p.nonIPX.Close()
	}
	p.mu.Unlock()
	p.Sink.Close()
	return nil
}

func (p *Phys) Run() error {
	for {
		pkt, err := p.ps.NextPacket()
		if err != nil {
			return err
		}
		payload, ok := Unframe(pkt, p.Sink.framer)
		if ok {
			ipxpkt := &ipx.Packet{}
			if err := ipxpkt.UnmarshalBinary(payload); err != nil {
				return err
			}
			// We discard looped-back packets (bug #18):
			if ipxpkt.Header.TransControl != loopbackDetectValue {
				p.rxpipe.WritePacket(ipxpkt)
			}
		} else {
			p.mu.Lock()
			if p.nonIPX != nil {
				p.nonIPX.frames <- pkt
			}
			p.mu.Unlock()
		}
	}
}

// ReadPacket implements the ipx.Reader interface, and will block until an
// IPX packet is read from the physical interface.
func (p *Phys) ReadPacket(ctx context.Context) (*ipx.Packet, error) {
	return p.rxpipe.ReadPacket(ctx)
}

// NonIPX returns a DuplexEthernetStream from which all non-IPX Ethernet frames
// will be returned by ReadPacketData().
func (p *Phys) NonIPX() DuplexEthernetStream {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.nonIPX == nil {
		p.nonIPX = &nonIPX{
			phys:   p,
			frames: make(chan gopacket.Packet),
			sb:     gopacket.NewSerializeBuffer(),
		}
	}
	return p.nonIPX
}

type nonIPX struct {
	phys   *Phys
	frames chan gopacket.Packet
	sb     gopacket.SerializeBuffer
}

func (ni *nonIPX) serializePacket(pkt gopacket.Packet) ([]byte, error) {
	// We got a packet. But we recompute checksums when converting back
	// into a byte slice (rather than just calling pkt.Data()). The
	// reason is that if we are capturing from a physical interface,
	// hardware checksum offloading in the kernel may mean that the
	// checksums are invalid. In particular I found problems with the
	// Linux veth devices where checksumming is skipped entirely since
	// it's not usually needed.
	ls := pkt.Layers()
	eth, ok := ls[0].(*layers.Ethernet)
	if !ok {
		return nil, fmt.Errorf("non-ethernet frame (this should not happen")
	}
	newLayers := []gopacket.SerializableLayer{eth}

	// This is deliberately hard-coded so that we only ever do CRC
	// recompute for IP, TCP and UDP - nothing else. If gopacket's
	// serialization of higher-level layers is used, it will change the
	// contents of some protocols.
	if ip, ok := ls[1].(*layers.IPv4); ok {
		newLayers = append(newLayers, ip)
		if udp, ok := ls[2].(*layers.UDP); ok {
			newLayers = append(newLayers, udp)
			udp.SetNetworkLayerForChecksum(ip)
		} else if tcp, ok := ls[2].(*layers.TCP); ok {
			newLayers = append(newLayers, tcp)
			tcp.SetNetworkLayerForChecksum(ip)
		}
	}
	payload := newLayers[len(newLayers)-1].(gopacket.Layer).LayerPayload()
	newLayers = append(newLayers, gopacket.Payload(payload))

	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
	}
	if err := gopacket.SerializeLayers(ni.sb, opts, newLayers...); err != nil {
		return nil, err
	}
	return ni.sb.Bytes(), nil
}

func (ni *nonIPX) ReadPacketData() ([]byte, gopacket.CaptureInfo, error) {
	pkt, ok := <-ni.frames
	if !ok {
		return nil, gopacket.CaptureInfo{}, io.EOF
	}
	result, err := ni.serializePacket(pkt)
	if err != nil {
		return nil, gopacket.CaptureInfo{}, err
	}
	return result, pkt.Metadata().CaptureInfo, nil
}

func (ni *nonIPX) WritePacketData(frame []byte) error {
	// Write is just a passthrough to the underlying sink.
	return ni.phys.Sink.pds.WritePacketData(frame)
}

func (ni *nonIPX) Close() {
	ni.phys.mu.Lock()
	close(ni.frames)
	ni.phys.nonIPX = nil
	ni.phys.mu.Unlock()
}

func NewPhys(stream DuplexEthernetStream, framer Framer) *Phys {
	return &Phys{
		Sink:   NewSink(stream, framer),
		ps:     gopacket.NewPacketSource(stream, layers.LinkTypeEthernet),
		rxpipe: pipe.New(),
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
