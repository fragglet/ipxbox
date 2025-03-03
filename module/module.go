package module

import (
	"context"

	"github.com/fragglet/ipxbox/network"
)

// Module defines an interface for optional modules that can be part of
// an ipxbox server. A module can also be run standalone, connecting to
// a remote server instead.
type Module interface {
	// Initialize sets up the module, and in particular registers
	// any flags that it might use.
	Initialize()

	// IsEnabled returns true if this module should be started. The
	// module usually determines whether this is possible by checking
	// the command line flags it has registered.
	IsEnabled() bool

	// Start activates the module, using `net` as its connection to
	// the internal IPX network.
	Start(ctx context.Context, net network.Network)
}
