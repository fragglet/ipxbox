// Package virtual contains an implementation of an IPX network that
// forwards packets between nodes, similar to a network switch.
package virtual

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
)

type Network struct {
	mu           sync.RWMutex
	nodesByIPX   map[ipx.Addr]*node
	nextTapID    int
	taps         map[int]*Tap
	BlockNetBIOS bool
}

type Tap struct {
	net   *Network
	pipeR *io.PipeReader
	pipeW *io.PipeWriter
	id    int
}

type node struct {
	net   *Network
	addr  ipx.Addr
	pipeR *io.PipeReader
	pipeW *io.PipeWriter
}

var (
	_ = (network.Network)(&Network{})
	_ = (network.Node)(&node{})
	_ = (io.ReadWriteCloser)(&Tap{})

	// Well-known IPX ports used for NetBIOS/SMB.
	netbiosPorts = map[uint16]bool{
		0x451: true, // NCP
		0x452: true, // SAP
		0x453: true, // RIP
		0x455: true, // NetBIOS
		0x553: true, // NWLink datagram, may contain SMB
	}

	// UnknownNodeError is returned by Network.Write() if the destination
	// MAC address is not associated with any known node.
	UnknownNodeError = errors.New("unknown destination address")

	// FilteredPacketError is returned when the virtual network is
	// configured to filter packets of this type.
	FilteredPacketError = errors.New("packet filtered")
)

// Close removes the node from its parent network; future calls to Read() will
// return EOF and packets sent to its address will not be delivered.
func (n *node) Close() error {
	n.pipeW.Close()
	n.net.mu.Lock()
	delete(n.net.nodesByIPX, n.addr)
	n.net.mu.Unlock()
	return nil
}

// Read reads a packet from the network for this node.
func (n *node) Read(data []byte) (int, error) {
	return n.pipeR.Read(data)
}

// Write writes a packet into the network from the given node.
func (n *node) Write(packet []byte) (int, error) {
	return n.net.writeFromSource(packet, n)
}

// Address returns the address of the given node.
func (n *node) Address() ipx.Addr {
	return n.addr
}

// Close removes the tap from the network; no more packets will be delivered
// to it and all future calls to Read() will return EOF.
func (t *Tap) Close() error {
	t.pipeW.Close()
	t.net.mu.Lock()
	delete(t.net.taps, t.id)
	t.net.mu.Unlock()
	return nil
}

// Read reads a packet from the network tap.
func (t *Tap) Read(data []byte) (int, error) {
	return t.pipeR.Read(data)
}

// Write writes a packet into the network.
func (t *Tap) Write(packet []byte) (int, error) {
	return t.net.writeFromSource(packet, t)
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
	r, w := io.Pipe()
	node := &node{
		net:   n,
		pipeR: r,
		pipeW: w,
	}
	n.addNode(node)
	return node
}

// forwardBroadcastPacket takes a broadcast packet received from a node and
// forwards it to all other clients; however, it is never sent back to the
// source node from which it came.
func (n *Network) forwardBroadcastPacket(header *ipx.Header, packet []byte, src io.Writer) error {
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
		// owner of the node will receive it by calling Read() on the
		// node which reads from the other end of the pipe.
		_, err := node.pipeW.Write(packet)
		if err != nil {
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
func (n *Network) forwardToTaps(packet []byte, src io.Writer) {
	taps := []*Tap{}
	n.mu.RLock()
	for _, tap := range n.taps {
		if tap != src {
			taps = append(taps, tap)
		}
	}
	n.mu.RUnlock()
	for _, tap := range taps {
		tap.pipeW.Write(packet)
	}
}

// forwardPacket receives a packet and forwards it on to another node.
func (n *Network) forwardPacket(header *ipx.Header, packet []byte, src io.Writer) error {
	if n.BlockNetBIOS && (netbiosPorts[header.Dest.Socket] ||
		netbiosPorts[header.Src.Socket]) {
		return FilteredPacketError
	}

	n.forwardToTaps(packet, src)
	if header.IsBroadcast() {
		return n.forwardBroadcastPacket(header, packet, src)
	}

	// We can only forward it on if the destination IPX address corresponds
	// to a node that we know about:
	n.mu.RLock()
	node, ok := n.nodesByIPX[header.Dest.Addr]
	n.mu.RUnlock()
	if !ok {
		return UnknownNodeError
	}
	_, err := node.pipeW.Write(packet)
	return err
}

// writeFromSource writes a packet to the network, forwarding to the right
// node as appropriate.
func (n *Network) writeFromSource(packet []byte, src io.Writer) (int, error) {
	var header ipx.Header
	if err := header.UnmarshalBinary(packet); err != nil {
		return 0, err
	}
	if err := n.forwardPacket(&header, packet, src); err != nil {
		return 0, err
	}
	return len(packet), nil
}

// Tap creates a new network tap for listening to network traffic.
// The caller must call Read() on the tap regularly otherwise it may stall the
// operation of the network.
func (n *Network) Tap() *Tap {
	r, w := io.Pipe()
	n.mu.Lock()
	tap := &Tap{
		id:    n.nextTapID,
		net:   n,
		pipeR: r,
		pipeW: w,
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
