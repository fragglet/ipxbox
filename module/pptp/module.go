package pptp

import (
	"context"
	"flag"
	"fmt"

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

func (m *mod) Start(ctx context.Context, params *module.Parameters) error {
	if !*m.enabled {
		return module.NotNeeded
	}
	pptps, err := pptp.NewServer(params.Network)
	if err != nil {
		return fmt.Errorf("failed to start PPTP server: %v", err)
	}
	pptps.Run(ctx)
	return nil
}
