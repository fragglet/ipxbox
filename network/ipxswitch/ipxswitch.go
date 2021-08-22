// Package ipxswitch contains an implementation of an IPX network that
// emulates the behavior of an Ethernet hub (but IPX native).
// TODO: Emulate the behavior of an Ethernet switch.
package ipxswitch

import (
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
	nodesByID map[int]*node
	nextNodeID int
}

type node struct {
	net    *Network
	nodeID int
	rxpipe ipx.ReadWriteCloser
}

var (
	_ = (network.Network)(&Network{})
	_ = (network.Node)(&node{})

	// UnknownNodeError is returned by Network.WritePacket() if the
	// destination MAC address is not associated with any known node.
	UnknownNodeError = errors.New("unknown destination address")
)

// Close removes the node from its parent network; future calls to ReadPacket()
// will return EOF and packets sent to its address will not be delivered.
func (n *node) Close() error {
	n.net.mu.Lock()
	delete(n.net.nodesByID, n.nodeID)
	n.net.mu.Unlock()
	return n.rxpipe.Close()
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
	return false
}

// NewNode creates a new node on the network.
func (n *Network) NewNode() network.Node {
	node := &node{
		net:    n,
		rxpipe: pipe.New(numBufferedPackets),
	}
	n.mu.Lock()
	node.nodeID = n.nextNodeID
	n.nextNodeID++
	n.nodesByID[node.nodeID] = node
	n.mu.Unlock()
	return node
}

// forwardPacket receives a packet and forwards it on to another node.
func (n *Network) forwardPacket(packet *ipx.Packet, src ipx.Writer) error {
	nodes := []*node{}
	n.mu.RLock()
	for _, node := range n.nodesByID {
		if node != src {
			nodes = append(nodes, node)
		}
	}
	n.mu.RUnlock()
	errs := []string{}
	for _, node := range nodes {
		// Packet is written into the delivery pipe for the node; the
		// owner of the node will receive it by calling ReadPacket()
		// from the other end of the pipe.
		if err := node.rxpipe.WritePacket(packet); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors when forwarding packets: %v", strings.Join(errs, "; "))
	}
	return nil
}

// New creates a new Network.
func New() *Network {
	return &Network{
		nodesByID: map[int]*node{},
	}
}