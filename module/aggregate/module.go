// Package aggregate implements a server module that contains other server
// modules.
package aggregate

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/fragglet/ipxbox/module"
)

type mod struct {
	modules []module.Module
}

func (m *mod) Initialize() {
	for _, submod := range m.modules {
		submod.Initialize()
	}
}

func (m *mod) IsEnabled() bool {
	for _, submod := range m.modules {
		if submod.IsEnabled() {
			return true
		}
	}
	return false
}

func moduleRunner(ctx context.Context, m module.Module, params *module.Parameters) func() error {
	return func() error {
		m.Start(ctx, params)
		return fmt.Errorf("module %+v terminated", m)
	}
}

func (m *mod) Start(ctx context.Context, params *module.Parameters) error {
	eg, egctx := errgroup.WithContext(ctx)
	for _, submod := range m.modules {
		if submod.IsEnabled() {
			eg.Go(moduleRunner(egctx, submod, params))
		}
	}
	return eg.Wait()
}

func MakeModule(modules ...module.Module) module.Module {
	return &mod{
		modules: modules,
	}
}
