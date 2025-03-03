// Package network defines types to represent an IPX network.
package network

import (
	"log"

	"github.com/fragglet/ipxbox/ipx"
)

// Network represents the concept of an IPX network.
type Network interface {
	// NewNode creates a new network node. If the network is located on
	// the end of a network link, this function may block for some time
	// until it completes.
	NewNode() (Node, error)
}

// Node represents a node attached to an IPX network.
type Node interface {
	ipx.ReadWriteCloser

	// GetProperty populates the given value based on its type. Since
	// network implementations may consist of many layers, this will
	// query through the layers to fetch the property. If successful,
	// true is returned.
	GetProperty(value interface{}) bool
}

// NodeAddress returns the IPX address assigned too the given node, or it
// returns ipx.AddrNull if there is no assigned address.
func NodeAddress(n Node) ipx.Addr {
	var result ipx.Addr
	if !n.GetProperty(&result) {
		return ipx.AddrNull
	}
	return result
}

// MustMakeNode is a convenience function that calls `NewNode()` but aborts
// the program if it fails. This should only ever be used at program startup
// or in test code.
func MustMakeNode(net Network) Node {
	node, err := net.NewNode()
	if err != nil {
		log.Fatalf("failed to create network node: %v", err)
	}
	return node
}
