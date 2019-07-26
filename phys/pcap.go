// Package phys implements a physical packet interface that uses libpcap
// to send and receive packets on a physical network interface.
package phys

import (
	"fmt"
	"io"
	"net"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

const etherTypeIPX = layers.EthernetType(0x8137)

var (
	_ = (io.ReadWriteCloser)(&PcapPhys{})
)

type PcapPhys struct {
	handle *pcap.Handle
	ps     *gopacket.PacketSource
}

func NewPcap(handle *pcap.Handle) (*PcapPhys, error) {
	filter := fmt.Sprintf("ether proto %d", etherTypeIPX)
	if err := handle.SetBPFFilter(filter); err != nil {
		return nil, err
	}
	ps := gopacket.NewPacketSource(handle, handle.LinkType())
	return &PcapPhys{
		handle: handle,
		ps:     ps,
	}, nil
}

func (p *PcapPhys) Close() error {
	p.handle.Close()
	return nil
}

// getIPXPayload scans through layers of the given packet to find the Ethernet
// layer and returns the IPX payload if the packet contains one, otherwise nil.
func getIPXPayload(p gopacket.Packet) []byte {
	for _, l := range p.Layers() {
		e, ok := l.(*layers.Ethernet)
		if !ok || e.EthernetType != etherTypeIPX {
			continue
		}
		return e.LayerPayload()
	}
	return nil
}

// Read implements the io.Reader interface, and will block until an IPX packet
// is received from the pcap handle.
func (p *PcapPhys) Read(result []byte) (int, error) {
	for {
		p, err := p.ps.NextPacket()
		if err != nil {
			return 0, nil
		}
		pl := getIPXPayload(p)
		if pl != nil {
			cnt := len(pl)
			if len(result) < cnt {
				cnt = len(result)
			}
			copy(result[:cnt], pl[:cnt])
			return cnt, nil
		}
	}
}

// Write writes an ethernet frame to the pcap handle containing the given IPX
// packet as payload.
func (p *PcapPhys) Write(packet []byte) (int, error) {
	ipxHeader := &ipx.Header{}
	if err := ipxHeader.UnmarshalBinary(packet); err != nil {
		return 0, err
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{}
	gopacket.SerializeLayers(buf, opts,
		&layers.Ethernet{
			SrcMAC:       net.HardwareAddr(ipxHeader.Src.Addr[:]),
			DstMAC:       net.HardwareAddr(ipxHeader.Dest.Addr[:]),
			EthernetType: etherTypeIPX,
		},
		gopacket.Payload(packet),
	)
	err := p.handle.WritePacketData(buf.Bytes())
	if err != nil {
		return 0, err
	}
	return len(packet), nil
}
