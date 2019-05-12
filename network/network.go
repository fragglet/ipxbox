// Package network defines types to represent an IPX network.
package network

import (
	"io"

	"github.com/fragglet/ipxbox/ipx"
)

// Network represents the concept of an IPX network.
type Network interface {
	// NewNode creates a new network node.
	NewNode() Node
}

// Node represents a node attached to an IPX network.
type Node interface {
	io.ReadWriteCloser

	// Address returns the IPX address of the node.
	Address() ipx.Addr
}
