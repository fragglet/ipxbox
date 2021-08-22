// Package addressable implements a Network that wraps another Network
// but assigns a unique IPX address to each node and only allows packets
// to be written that have that source address.
package addressable

import (
	"crypto/rand"
	"errors"
	"sync"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
)

var (
	_ = (network.Network)(&addressableNetwork{})
	_ = (network.Node)(&node{})

	// WrongAddressError is returned when a packet is written with the
	// wrong source IPX address.
	WrongAddressError = errors.New("packet has wrong source address")
)

type addressableNetwork struct {
	inner      network.Network
	nodesByIPX map[ipx.Addr]*node
	mu         sync.Mutex
}

func (n *addressableNetwork) NewNode() network.Node {
	result := &node{net: n}
	// Repeatedly generate a new IPX address until we generate one that
	// is not already in use. A prefix of 02:... gives a Unicast address
	// that is locally administered.
	for {
		var addr ipx.Addr
		addr[0] = 0x02
		rand.Read(addr[1:])
		n.mu.Lock()
		if _, ok := n.nodesByIPX[addr]; !ok {
			result.addr = addr
			n.nodesByIPX[addr] = result
			n.mu.Unlock()
			break
		}
		n.mu.Unlock()
	}
	result.inner = n.inner.NewNode()
	return result
}

type node struct {
	net   *addressableNetwork
	inner network.Node
	addr  ipx.Addr
}

func (n *node) ReadPacket() (*ipx.Packet, error) {
	var packet *ipx.Packet
	for {
		var err error
		packet, err = n.inner.ReadPacket()
		if err != nil {
			return nil, err
		}
		dest := &packet.Header.Dest
		if dest.Network == ipx.ZeroNetwork {
			if dest.Addr == n.addr {
				break
			}
			if dest.Addr == ipx.AddrBroadcast {
				break
			}
		}
		// Keep looping until we find a packet that's really
		// destined for us.
	}
	return packet, nil
}

func (n *node) WritePacket(packet *ipx.Packet) error {
	src := &packet.Header.Src
	if src.Network != ipx.ZeroNetwork || src.Addr != n.addr {
		return WrongAddressError
	}
	return n.inner.WritePacket(packet)
}

func (n *node) Close() error {
	n.net.mu.Lock()
	delete(n.net.nodesByIPX, n.addr)
	n.net.mu.Unlock()
	return n.inner.Close()
}

func (n *node) GetProperty(x interface{}) bool {
	switch x.(type) {
	case *ipx.Addr:
		*x.(*ipx.Addr) = n.addr
		return true
	default:
		return n.inner.GetProperty(x)
	}
}

// Wrap creates a network that wraps the given network but assigns a unique
// IPX address to each node.
func Wrap(n network.Network) network.Network {
	return &addressableNetwork{
		inner:      n,
		nodesByIPX: map[ipx.Addr]*node{},
	}
}
