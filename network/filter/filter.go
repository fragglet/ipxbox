// Package filter implements a network that wraps another network but drops
// packets using well-known ports.
package filter

import (
	"context"
	"errors"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
)

var (
	_ = (network.Network)(&filteringNetwork{})
	_ = (network.Node)(&filter{})

	// Well-known IPX ports used for NetBIOS/SMB.
	netbiosPorts = map[uint16]bool{
		0x451:  true, // NCP
		0x452:  true, // SAP
		0x453:  true, // RIP
		0x455:  true, // NetBIOS
		0x551:  true, // NWLink SMB Name Query
		0x552:  true, // NWLink SMB Redirector
		0x553:  true, // NWLink datagram, may contain SMB
		0x900F: true, // SNMP over IPX, RFC 1298
		0x9010: true, // SNMP over IPX, RFC 1298
	}

	// FilteredPacketError is returned when the virtual network is
	// configured to filter packets of this type.
	FilteredPacketError = errors.New("packet filtered")
)

type filter struct {
	inner ipx.ReadWriteCloser
}

func shouldFilter(hdr *ipx.Header) bool {
	return netbiosPorts[hdr.Dest.Socket] || netbiosPorts[hdr.Src.Socket]
}

func (f *filter) ReadPacket(ctx context.Context) (*ipx.Packet, error) {
	for {
		packet, err := f.inner.ReadPacket(ctx)
		if err != nil {
			return nil, err
		}
		if !shouldFilter(&packet.Header) {
			return packet, nil
		}
	}
}

func (f *filter) WritePacket(packet *ipx.Packet) error {
	if shouldFilter(&packet.Header) {
		return FilteredPacketError
	}
	return f.inner.WritePacket(packet)
}

func (f *filter) Close() error {
	return f.inner.Close()
}

func (f *filter) GetProperty(x interface{}) bool {
	if node, ok := f.inner.(network.Node); ok {
		return node.GetProperty(x)
	}
	return false
}

type filteringNetwork struct {
	inner network.Network
}

func (n *filteringNetwork) NewNode() network.Node {
	return &filter{inner: n.inner.NewNode()}
}

// Wrap creates a network that wraps the given network but rejects packets
// using certain well-known port numbers which could present a security risk.
func Wrap(n network.Network) network.Network {
	return &filteringNetwork{inner: n}
}

// New creates a new ReadWriteCloser that wraps the given ReadWriteCloser
// but discards packets using well-known port numbers.
func New(inner ipx.ReadWriteCloser) ipx.ReadWriteCloser {
	return &filter{inner: inner}
}
