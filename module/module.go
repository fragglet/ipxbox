package module

import (
	"context"
	"errors"
	"log"

	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/phys"
)

// Module defines an interface for optional modules that can be part of
// an ipxbox server. A module can also be run standalone, connecting to
// a remote server instead.
type Module interface {
	// Initialize sets up the module, and in particular registers
	// any flags that it might use.
	Initialize()

	// Start activates the module.
	Start(ctx context.Context, params *Parameters) error
}

type Parameters struct {
	// Network is the connection to the IPX network that the module should
	// use for communications.
	Network network.Network

	// Uplinkable is an IPX network implementation that the module can use
	// for direct connection into the network.
	Uplinkable network.Network

	// Logger should be used for reporting log messages.
	Logger *log.Logger

	// Phys represents a bridge to a real, physical network, if one is configured.
	Phys *phys.Phys
}

var (
	NotNeeded = errors.New("module exited with nothing to do")
)
