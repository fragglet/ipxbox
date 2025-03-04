package ipxpkt

import (
	"context"

	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/phys"
)

type mod struct {
	bridge  *phys.Spec
}

var (
	Module = &mod{}
	_      = (module.Module)(Module)
)

func (m *mod) Initialize() {
	m.bridge = phys.SpecFlag("ipxpkt_bridge", "slirp", `Network connection for ipxpkt driver; the syntax is the same as the -bridge flag (default: "slirp").`)
}

func (m *mod) Start(ctx context.Context, params *module.Parameters) error {
	port := network.MustMakeNode(params.Network)
	r := NewRouter(port)

	tapConn, err := m.bridge.EthernetStream(true)
	if err != nil {
		return err
	}
	tapConn = phys.NewChecksumFixer(tapConn)

	return phys.CopyFrames(r, tapConn)
}
