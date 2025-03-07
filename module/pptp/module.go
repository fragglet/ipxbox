package pptp

import (
	"context"
	"fmt"

	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/ppp/pptp"
)

type mod struct{}

var (
	Module = (module.Module)(&mod{})
)

func (m *mod) Initialize() {
}

func (m *mod) Start(ctx context.Context, params *module.Parameters) error {
	pptps, err := pptp.NewServer(params.Network)
	if err != nil {
		return fmt.Errorf("failed to start PPTP server: %v", err)
	}
	pptps.Run(ctx)
	return nil
}
