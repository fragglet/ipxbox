// Package phys implements a physical packet interface that uses libpcap
// to send and receive packets on a physical network interface.
package phys

import (
	"io"
	"net"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
)

var (
	_ = (io.ReadWriteCloser)(&PcapPhys{})
)

// packetDuplexStream represents the concept of a two-way stream of packets
// where packets can be both read from and written to the stream.
type packetDuplexStream interface {
	gopacket.PacketDataSource

	Close()
	WritePacketData([]byte) error
}

type PcapPhys struct {
	stream packetDuplexStream
	ps     *gopacket.PacketSource
	framer Framer
}

func NewPcap(handle *pcap.Handle, framer Framer) (*PcapPhys, error) {
	if err := handle.SetBPFFilter("ipx"); err != nil {
		return nil, err
	}
	ps := gopacket.NewPacketSource(handle, handle.LinkType())
	return &PcapPhys{
		stream: handle,
		ps:     ps,
		framer: framer,
	}, nil
}

func (p *PcapPhys) Close() error {
	p.stream.Close()
	return nil
}

// Read implements the io.Reader interface, and will block until an IPX packet
// is received from the pcap handle.
func (p *PcapPhys) Read(result []byte) (int, error) {
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

// Write writes an ethernet frame to the pcap handle containing the given IPX
// packet as payload.
func (p *PcapPhys) Write(packet []byte) (int, error) {
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
