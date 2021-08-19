// Package filter implements a network that wraps another network but drops
// packets using well-known ports.
package filter

import (
	"errors"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
)

var (
	_ = (network.Network)(&filteringNetwork{})
	_ = (network.Node)(&node{})

	// Well-known IPX ports used for NetBIOS/SMB.
	netbiosPorts = map[uint16]bool{
		0x451:  true, // NCP
		0x452:  true, // SAP
		0x453:  true, // RIP
		0x455:  true, // NetBIOS
		0x553:  true, // NWLink datagram, may contain SMB
		0x900F: true, // SNMP over IPX, RFC 1298
		0x9010: true, // SNMP over IPX, RFC 1298
	}

	// FilteredPacketError is returned when the virtual network is
	// configured to filter packets of this type.
	FilteredPacketError = errors.New("packet filtered")
)

type filteringNetwork struct {
	inner network.Network
}

func (n *filteringNetwork) NewNode() network.Node {
	return &node{inner: n.inner.NewNode()}
}

type node struct {
	inner network.Node
}

func (n *node) ReadPacket() (*ipx.Packet, error) {
	return n.inner.ReadPacket()
}

func (n *node) WritePacket(packet *ipx.Packet) error {
	if netbiosPorts[packet.Header.Dest.Socket] || netbiosPorts[packet.Header.Src.Socket] {
		return FilteredPacketError
	}
	return n.inner.WritePacket(packet)
}

func (n *node) Close() error {
	return n.inner.Close()
}

func (n *node) GetProperty(x interface{}) bool {
	return n.inner.GetProperty(x)
}

func (n *node) Address() ipx.Addr {
	return n.inner.Address()
}

// New creates a network that wraps the given network but rejects packets
// using certain well-known port numbers which could present a security risk.
func New(n network.Network) network.Network {
	return &filteringNetwork{inner: n}
}
