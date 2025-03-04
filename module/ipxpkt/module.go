package ipxpkt

import (
	"context"
	"flag"

	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/phys"
)

type mod struct {
	enabled *bool
	bridge  *phys.Spec
}

var (
	Module = &mod{}
	_      = (module.Module)(Module)
)

func (m *mod) Initialize() {
	m.enabled = flag.Bool("enable_ipxpkt", false, "If true, route encapsulated packets from the IPXPKT.COM driver to the physical network")
	m.bridge = phys.SpecFlag("ipxpkt_bridge", "slirp", `Network connection for ipxpkt driver; the syntax is the same as the -bridge flag (default: "slirp").`)
}

func (m *mod) Start(ctx context.Context, params *module.Parameters) error {
	if !*m.enabled {
		return module.NotNeeded
	}

	port := network.MustMakeNode(params.Network)
	r := NewRouter(port)

	tapConn, err := m.bridge.EthernetStream(true)
	if err != nil {
		return err
	}
	tapConn = phys.NewChecksumFixer(tapConn)

	return phys.CopyFrames(r, tapConn)
}
