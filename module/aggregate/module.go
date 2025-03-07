// Package aggregate implements a server module that contains other server
// modules.
package aggregate

import (
	"context"
	"log"
	"reflect"

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

func moduleRunner(ctx context.Context, m module.Module, params *module.Parameters) func() error {
	return func() error {
		err := m.Start(ctx, params)
		if err == module.NotNeeded {
			return nil
		} else if err == nil {
			err = module.EarlyTermination
		}
		// TODO: It ought to be the case that the cancelled context
		// from a failed module causes all the other modules to shut
		// down. However, not all modules reliably shut down when
		// cancelled yet. Instead, we quit with a fatal abort on the
		// first detected failure.
		log.Fatalf("module %s terminated unexpectedly: %v", reflect.TypeOf(m).String(), err)
		return err
	}
}

func (m *mod) Start(ctx context.Context, params *module.Parameters) error {
	eg, egctx := errgroup.WithContext(ctx)
	for _, submod := range m.modules {
		eg.Go(moduleRunner(egctx, submod, params))
	}
	err := eg.Wait()
	if err == nil {
		return module.NotNeeded
	}
	return err
}

func MakeModule(modules ...module.Module) module.Module {
	return &mod{
		modules: modules,
	}
}
