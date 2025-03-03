// Package bridge is a module that taps the internal network and bridges it
// to a physical network if one is configured.
package bridge

import (
	"context"

	"golang.org/x/sync/errgroup"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/network"
)

type mod struct {
}

var (
	Module = (module.Module)(&mod{})
)

func (m *mod) Initialize() {
}

func (m *mod) Start(ctx context.Context, params *module.Parameters) error {
	if params.Phys == nil || params.Uplinkable == nil {
		return module.NotNeeded
	}

	port := network.MustMakeNode(params.Uplinkable)
	eg, egctx := errgroup.WithContext(ctx)
	eg.Go(params.Phys.Run)
	eg.Go(func() error {
	      return ipx.DuplexCopyPackets(egctx, params.Phys, port)
	})
	return eg.Wait()
}
