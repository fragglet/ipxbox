package pptp

import (
	"context"
	"flag"
	"log"

	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/ppp/pptp"
)

type mod struct {
	enabled *bool
}

var (
	Module = (module.Module)(&mod{})
)

func (m *mod) Initialize() {
	m.enabled = flag.Bool("enable_pptp", false, "If true, run PPTP VPN server on TCP port 1723.")
}

func (m *mod) IsEnabled() bool {
	return *m.enabled
}

func (m *mod) Start(ctx context.Context, params *module.Parameters) error {
	pptps, err := pptp.NewServer(params.Network)
	if err != nil {
		log.Fatalf("failed to start PPTP server: %v", err)
	}
	go pptps.Run(ctx)
	return nil
}
