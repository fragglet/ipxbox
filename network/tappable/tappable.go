// Package tappable implements a network that wraps another network
// but also allows network taps to snoop on network traffic.
package tappable

import (
	"context"
	"sync"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/network/pipe"
)

var (
	_ = (network.Network)(&TappableNetwork{})
	_ = (network.Node)(&node{})
	_ = (ipx.ReadCloser)(&tap{})
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

func (n *TappableNetwork) NewTap() ipx.ReadCloser {
	n.mu.Lock()
	defer n.mu.Unlock()
	tap := &tap{
		net:    n,
		rxpipe: pipe.New(),
		tapID:  n.nextTapID,
	}
	n.nextTapID++
	n.taps[tap.tapID] = tap
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

func (n *node) ReadPacket(ctx context.Context) (*ipx.Packet, error) {
	return n.inner.ReadPacket(ctx)
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
	rxpipe ipx.ReadWriteCloser
	net    *TappableNetwork
	tapID  int
}

func (t *tap) ReadPacket(ctx context.Context) (*ipx.Packet, error) {
	return t.rxpipe.ReadPacket(ctx)
}

func (t *tap) Close() error {
	t.net.deleteTap(t.tapID)
	return t.rxpipe.Close()
}

// Wrap creates a TappableNetwork that wraps another network but also allows
// taps that can be used to snoop on traffic.
func Wrap(n network.Network) *TappableNetwork {
	return &TappableNetwork{
		inner: n,
		taps:  make(map[int]*tap),
	}
}
