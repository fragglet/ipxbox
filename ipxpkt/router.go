// Package ipxpkt implements a packet router that wraps Ethernet frames in
// IPX packets using the protocol from the IPXPKT.COM DOS packet driver.
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

// Router implements the ipxpkt protocol and implements the same
// DuplexEthernetStream interface as a real physical Ethernet link;
// it communicates by sending and receiving IPX packets.
type Router struct {
	node          network.Node
	packetCounter uint16
	fr            frameReassembler
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
		return nil, fmt.Errorf("not an ipxpkt fragment; destination socket %d != %d", ipxHeader.Dest.Socket, ipxSocket)
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
	frame, complete := r.fr.reassemble(&ipxHeader, &hdr, packet[HeaderLength:])
	if !complete {
		return nil, fmt.Errorf("incomplete frame")
	}
	return frame, nil
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
		Checksum: 0xffff,
	}
	// TODO: Hardware address from Ethernet frame may not match the IPX
	// address to forward to. This needs a routing table implementation
	// equivalent to what ipxpkt does.
	copy(hdr1.Dest.Addr[:], frame[0:6])

	r.packetCounter++
	fragments := fragmentFrame(frame)

	hdr2 := &Header{
		NumFragments: uint8(len(fragments)),
		PacketID:     r.packetCounter,
	}

	for fragIndex, frag := range fragments {
		hdr1.Length = uint16(ipx.HeaderLength + HeaderLength + trailBytes + len(frag))
		data, err := hdr1.MarshalBinary()
		if err != nil {
			return err
		}

		// TODO: Support non-trail version of ipxpkt
		var trail [trailBytes]byte
		data = append(data, trail[:]...)

		hdr2.Fragment = uint8(fragIndex + 1)
		data2, err := hdr2.MarshalBinary()
		if err != nil {
			return err
		}

		data = append(data, data2...)
		data = append(data, frag...)
		if _, err := r.node.Write(data); err != nil {
			// Failure here is not necessarily an error; there may
			// not be any appropriate destination for the packet.
			// But don't bother sending the other fragments.
			return nil
		}
	}
	return nil
}

func NewRouter(node network.Node) *Router {
	r := &Router{
		node: node,
	}
	r.fr.init()
	return r
}
