// Package tappable implements a network that wraps another network
// but also allows network taps to snoop on network traffic.
package tappable

import (
	"sync"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/network/pipe"
)

const (
	// numBufferedPackets is the number of packets to buffer per
	// tap before we start dropping data. Tap readers should be
	// fast enough to ensure this never happens.
	numBufferedPackets = 16
)

var (
	_ = (network.Network)(&TappableNetwork{})
	_ = (network.Node)(&node{})
	// TODO: Change taps to be read-only.
	_ = (ipx.ReadWriteCloser)(&tap{})
)

type TappableNetwork struct {
	inner     network.Network
	nextTapID int
	taps      map[int]*tap
	mu        sync.RWMutex
}

func (n *TappableNetwork) NewNode() network.Node {
	return &node{
		net:   n,
		inner: n.inner.NewNode(),
	}
}

func (n *TappableNetwork) NewTap() ipx.ReadWriteCloser {
	n.mu.Lock()
	defer n.mu.Unlock()
	tap := &tap{
		net:    n,
		node:   n.inner.NewNode(),
		rxpipe: pipe.New(numBufferedPackets),
		tapID:  n.nextTapID,
	}
	n.nextTapID++
	n.taps[tap.tapID] = tap
	// We create an inner node for the tap, but just so that we can
	// inject packets via WritePacket(). Any delivered packets just
	// get read and discarded.
	go func() {
		for {
			_, err := tap.node.ReadPacket()
			if err != nil {
				break
			}
		}
	}()
	return tap
}

func (n *TappableNetwork) deleteTap(tapID int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.taps, tapID)
}

func (n *TappableNetwork) writeToTaps(packet *ipx.Packet) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	for _, tap := range n.taps {
		tap.rxpipe.WritePacket(packet)
	}
}

type node struct {
	inner network.Node
	net   *TappableNetwork
}

func (n *node) ReadPacket() (*ipx.Packet, error) {
	return n.inner.ReadPacket()
}

func (n *node) WritePacket(packet *ipx.Packet) error {
	n.net.writeToTaps(packet)
	return n.inner.WritePacket(packet)
}

func (n *node) Close() error {
	return n.inner.Close()
}

func (n *node) GetProperty(x interface{}) bool {
	return n.inner.GetProperty(x)
}

type tap struct {
	node   network.Node
	rxpipe ipx.ReadWriteCloser
	net    *TappableNetwork
	tapID  int
}

func (t *tap) ReadPacket() (*ipx.Packet, error) {
	return t.rxpipe.ReadPacket()
}

func (t *tap) WritePacket(packet *ipx.Packet) error {
	return t.node.WritePacket(packet)
}

func (t *tap) Close() error {
	t.net.deleteTap(t.tapID)
	t.rxpipe.Close()
	return t.node.Close()
}

// Wrap creates a TappableNetwork that wraps another network but also allows
// taps that can be used to snoop on traffic.
func Wrap(n network.Network) *TappableNetwork {
	return &TappableNetwork{
		inner: n,
		taps:  make(map[int]*tap),
	}
}
