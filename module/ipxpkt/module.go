package ipxpkt

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/phys"
)

type mod struct {
	enabled *bool
}

var (
	Module = &mod{}
	_      = (module.Module)(Module)
)

func (m *mod) Initialize() {
	m.enabled = flag.Bool("enable_ipxpkt", false, "If true, route encapsulated packets from the IPXPKT.COM driver to the physical network")
}

func (m *mod) Start(ctx context.Context, params *module.Parameters) error {
	if !*m.enabled {
		return module.NotNeeded
	}

	port := network.MustMakeNode(params.Network)
	r := NewRouter(port)

	var tapConn phys.DuplexEthernetStream
	var err error
	if params.Phys != nil {
		tapConn = params.Phys.NonIPX()
		log.Printf("Using physical network tap for ipxpkt router")
	} else {
		tapConn, err = phys.MakeSlirp()
		if err != nil {
			return fmt.Errorf("failed to open libslirp subprocess: %v.\nYou may need to install libslirp-helper, or alternatively use -bridge.", err)
		}
		log.Printf("Using Slirp subprocess for ipxpkt router")
	}

	return phys.CopyFrames(r, tapConn)
}

func (m *mod) Enabled() bool {
	return *m.enabled
}
