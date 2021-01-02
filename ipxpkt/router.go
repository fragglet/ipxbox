package ipxpkt

import (
	"fmt"
	"time"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/phys"

	"github.com/google/gopacket"
)

const (
	ipxSocket  = 0x6181
	trailBytes = 32
)

var (
	_ = (phys.DuplexEthernetStream)(&Router{})
)

type Router struct {
	node          network.Node
	packetCounter uint16
}

func (r *Router) Close() {
	r.node.Close()
}

func (r *Router) unwrapFrame(packet []byte) ([]byte, error) {
	var ipxHeader ipx.Header
	if err := ipxHeader.UnmarshalBinary(packet); err != nil {
		return nil, err
	}
	if ipxHeader.Dest.Socket != ipxSocket {
		return nil, fmt.Errorf("not an ipxpkt fragment; destination socket %d != %s", ipxHeader.Dest.Socket, ipxSocket)
	}
	packet = packet[ipx.HeaderLength:]

	// TODO: Support ipxpkt version without trail bytes
	if len(packet) < trailBytes+HeaderLength {
		return nil, fmt.Errorf("inner packet too short: %d < %d", len(packet), trailBytes+HeaderLength)
	}
	packet = packet[trailBytes:]

	var hdr Header
	if err := hdr.UnmarshalBinary(packet); err != nil {
		return nil, err
	}
	// TODO: Fragment reassembly
	if hdr.Fragment != 1 || hdr.NumFragments != 1 {
		return nil, fmt.Errorf("fragment reassembly not implemented yet")
	}
	return packet[HeaderLength:], nil
}

// readFrame reads an Ethernet frame from the router; it will block until
// a complete frame arrives from another node.
func (r *Router) ReadPacketData() ([]byte, gopacket.CaptureInfo, error) {
	var readBuf [1500]byte
	for {
		cnt, err := r.node.Read(readBuf[:])
		if err != nil {
			return nil, gopacket.CaptureInfo{}, err
		}
		frame, err := r.unwrapFrame(readBuf[:cnt])
		if err != nil {
			// TODO: Log error?
			continue
		}
		ci := gopacket.CaptureInfo{
			Timestamp:     time.Now(),
			CaptureLength: len(frame),
			Length:        len(frame),
		}
		return frame, ci, nil
	}
}

// WritePacketData writes an Ethernet frame to the router; this will be
// wrapped and fragmented into one or more ipxpkt frames and written to the
// IPX network.
func (r *Router) WritePacketData(frame []byte) error {
	hdr1 := &ipx.Header{
		Src: ipx.HeaderAddr{
			Addr:   r.node.Address(),
			Socket: ipxSocket,
		},
		Dest: ipx.HeaderAddr{
			// Addr: - is set below
			Socket: ipxSocket,
		},
		Length:   uint16(ipx.HeaderLength + HeaderLength + trailBytes + len(frame)),
		Checksum: 0xffff,
	}
	copy(hdr1.Dest.Addr[:], frame[0:6])
	data, err := hdr1.MarshalBinary()
	if err != nil {
		return err
	}

	// TODO: Support non-trail version of ipxpkt
	var trail [trailBytes]byte
	data = append(data, trail[:]...)

	// TODO: fragmentation for large packets
	hdr2 := &Header{
		Fragment:     1,
		NumFragments: 1,
		PacketID:     r.packetCounter,
	}
	data2, err := hdr2.MarshalBinary()
	if err != nil {
		return err
	}
	r.packetCounter++

	data = append(data, data2...)
	data = append(data, frame...)
	if _, err := r.node.Write(data); err != nil {
		// Failure here is not necessarily an error; there may just not
		// be any appropriate destination for the packet.
		//return err
	}
	return nil
}

func NewRouter(node network.Node) *Router {
	r := &Router{
		node: node,
	}
	return r
}
