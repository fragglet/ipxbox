// Package bridge is a module that taps the internal network and bridges it
// to a physical network if one is configured.
package bridge

import (
	"context"

	"golang.org/x/sync/errgroup"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/phys"
)

type mod struct {
	Bridge          *phys.Spec
	EthernetFraming *phys.Framer
}

var (
	Module = (module.Module)(&mod{})
)

func (m *mod) Initialize() {
	m.Bridge = phys.SpecFlag("bridge", "", `Bridge to physical network. Valid values are: "tap:" or "pcap:{device name}"`)
	m.EthernetFraming = phys.FramingTypeFlag("ethernet_framing", `Framing to use when sending Ethernet packets. Valid values are "auto", "802.2", "802.3raw", "snap" and "eth-ii".`)
}

func (m *mod) Start(ctx context.Context, params *module.Parameters) error {
	if params.Uplinkable == nil {
		return module.NotNeeded
	}
	stream, err := m.Bridge.EthernetStream(false)
	if err != nil {
		return err
	}
	if stream == nil {
		return module.NotNeeded
	}
	p := phys.NewPhys(stream, *m.EthernetFraming)

	port := network.MustMakeNode(params.Uplinkable)
	eg, egctx := errgroup.WithContext(ctx)
	eg.Go(p.Run)
	eg.Go(func() error {
		return ipx.DuplexCopyPackets(egctx, p, port)
	})
	return eg.Wait()
}
