// Package virtual contains an implementation of an IPX network that
// forwards packets between nodes, similar to a network switch.
package virtual

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/network/pipe"
)

const (
	// numBufferedPackets is the number of packets that we will
	// buffer per receive pipe before writes start to fail. This
	// is deliberately set fairly small because readers ought
	// to be fast enough that the buffers never become full and
	// we don't want buffer bloat.
	numBufferedPackets = 4
)

type Network struct {
	mu         sync.RWMutex
	nodesByIPX map[ipx.Addr]*node
	nextTapID  int
	taps       map[int]*Tap
}

type Tap struct {
	net    *Network
	rxpipe ipx.ReadWriteCloser
	id     int
}

type node struct {
	net    *Network
	addr   ipx.Addr
	rxpipe ipx.ReadWriteCloser
}

var (
	_ = (network.Network)(&Network{})
	_ = (network.Node)(&node{})
	_ = (ipx.ReadWriteCloser)(&Tap{})

	// UnknownNodeError is returned by Network.WritePacket() if the
	// destination MAC address is not associated with any known node.
	UnknownNodeError = errors.New("unknown destination address")
)

// Close removes the node from its parent network; future calls to ReadPacket()
// will return EOF and packets sent to its address will not be delivered.
func (n *node) Close() error {
	n.rxpipe.Close()
	n.net.mu.Lock()
	delete(n.net.nodesByIPX, n.addr)
	n.net.mu.Unlock()
	return nil
}

// ReadPacket reads a packet from the network for this node.
func (n *node) ReadPacket() (*ipx.Packet, error) {
	return n.rxpipe.ReadPacket()
}

// WritePacket writes a packet into the network from the given node.
func (n *node) WritePacket(packet *ipx.Packet) error {
	return n.net.forwardPacket(packet, n)
}

func (n *node) GetProperty(x interface{}) bool {
	switch x.(type) {
	case *ipx.Addr:
		*x.(*ipx.Addr) = n.addr
		return true
	default:
		return false
	}
}

// Close removes the tap from the network; no more packets will be delivered
// to it and all future calls to ReadPacket() will return EOF.
func (t *Tap) Close() error {
	t.rxpipe.Close()
	t.net.mu.Lock()
	delete(t.net.taps, t.id)
	t.net.mu.Unlock()
	return nil
}

// ReadPacket reads a packet from the network tap.
func (t *Tap) ReadPacket() (*ipx.Packet, error) {
	return t.rxpipe.ReadPacket()
}

// WritePacket writes a packet into the network.
func (t *Tap) WritePacket(packet *ipx.Packet) error {
	return t.net.forwardPacket(packet, t)
}

// addNode adds a new node to the network, setting its address to an unused
// address.
func (n *Network) addNode(node *node) {
	// Repeatedly generate a new IPX address until we generate one that
	// is not already in use. A prefix of 02:... gives a Unicast address
	// that is locally administered.
	for {
		var addr ipx.Addr
		addr[0] = 0x02
		rand.Read(addr[1:])
		n.mu.Lock()
		if _, ok := n.nodesByIPX[addr]; !ok {
			node.addr = addr
			n.nodesByIPX[addr] = node
			n.mu.Unlock()
			return
		}
		n.mu.Unlock()
	}
}

// NewNode creates a new node on the network.
func (n *Network) NewNode() network.Node {
	node := &node{
		net:    n,
		rxpipe: pipe.New(numBufferedPackets),
	}
	n.addNode(node)
	return node
}

// forwardBroadcastPacket takes a broadcast packet received from a node and
// forwards it to all other clients; however, it is never sent back to the
// source node from which it came.
func (n *Network) forwardBroadcastPacket(packet *ipx.Packet, src ipx.Writer) error {
	errs := []string{}
	nodes := []*node{}
	n.mu.RLock()
	for _, node := range n.nodesByIPX {
		if node != src {
			nodes = append(nodes, node)
		}
	}
	n.mu.RUnlock()
	for _, node := range nodes {
		// Packet is written into the delivery pipe for the node; the
		// owner of the node will receive it by calling ReadPacket()
		// from the other end of the pipe.
		if err := node.rxpipe.WritePacket(packet); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors when forwarding broadcast packets: %v", strings.Join(errs, "; "))
	}
	return nil
}

// forwardToTaps sends the given packet to all taps which are currently
// listening to network traffic. We don't forward packets back to the source
// that sent them, though.
func (n *Network) forwardToTaps(packet *ipx.Packet, src ipx.Writer) {
	taps := []*Tap{}
	n.mu.RLock()
	for _, tap := range n.taps {
		if tap != src {
			taps = append(taps, tap)
		}
	}
	n.mu.RUnlock()
	for _, tap := range taps {
		tap.rxpipe.WritePacket(packet)
	}
}

// forwardPacket receives a packet and forwards it on to another node.
func (n *Network) forwardPacket(packet *ipx.Packet, src ipx.Writer) error {
	n.forwardToTaps(packet, src)
	if packet.Header.IsBroadcast() {
		return n.forwardBroadcastPacket(packet, src)
	}

	// We can only forward it on if the destination IPX address corresponds
	// to a node that we know about:
	n.mu.RLock()
	node, ok := n.nodesByIPX[packet.Header.Dest.Addr]
	n.mu.RUnlock()
	if !ok {
		return UnknownNodeError
	}
	return node.rxpipe.WritePacket(packet)
}

// Tap creates a new network tap for listening to network traffic.
// The caller must call ReadPacket() on the tap regularly otherwise not all
// tapped packets may be captured.
func (n *Network) Tap() *Tap {
	n.mu.Lock()
	tap := &Tap{
		id:     n.nextTapID,
		net:    n,
		rxpipe: pipe.New(numBufferedPackets),
	}
	n.nextTapID++
	n.taps[tap.id] = tap
	n.mu.Unlock()
	return tap
}

// New creates a new Network.
func New() *Network {
	return &Network{
		nodesByIPX: map[ipx.Addr]*node{},
		taps:       map[int]*Tap{},
	}
}
