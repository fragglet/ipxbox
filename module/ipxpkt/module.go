package ipxpkt

import (
	"context"
	"flag"
	"log"

	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/phys"
)

type mod struct {
	enabled *bool
}

var (
	Module = (module.Module)(&mod{})
)

func (m *mod) Initialize() {
	m.enabled = flag.Bool("enable_ipxpkt", false, "If true, route encapsulated packets from the IPXPKT.COM driver to the physical network")
}

func (m *mod) IsEnabled() bool {
	return *m.enabled
}

func (m *mod) Start(ctx context.Context, params *module.Parameters) error {
	port := network.MustMakeNode(params.Network)
	r := NewRouter(port)
	// TODO: Add back option for bridge to physical network
	tapConn, err := phys.MakeSlirp()
	if err != nil {
		log.Fatalf("failed to open libslirp subprocess: %v.\nYou may need to install libslirp-helper, or alternatively use -enable_tap or -pcap_device.", err)
	}
	log.Printf("Using Slirp subprocess for ipxpkt router")
	go phys.CopyFrames(r, tapConn)
	return nil
}
