package qproxy

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/network"
)

const (
	clientTimeout = 10 * time.Minute
)

type mod struct {
	quakeServers *string
}

var (
	Module = (module.Module)(&mod{})
)

func (m *mod) Initialize() {
	m.quakeServers = flag.String("quake_servers", "", "Proxy to the given list of Quake UDP servers in a way that makes them accessible over IPX.")
}

func (m *mod) IsEnabled() bool {
	return *m.quakeServers != ""
}

func proxyRunner(ctx context.Context, p *Proxy, addr string) func() error {
	return func() error {
		p.Run(ctx)
		return fmt.Errorf("proxy to quake server %v terminated", addr)
	}
}

func (m *mod) Start(ctx context.Context, params *module.Parameters) error {
	eg, egctx := errgroup.WithContext(ctx)
	for _, addr := range strings.Split(*m.quakeServers, ",") {
		node := network.MustMakeNode(params.Network)
		p := New(&Config{
			Address:     addr,
			IdleTimeout: clientTimeout,
		}, node)
		eg.Go(proxyRunner(egctx, p, addr))
	}
	return eg.Wait()
}
