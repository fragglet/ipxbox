package qproxy

import (
	"context"
	"flag"
	"strings"
	"time"

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

func (m *mod) Start(ctx context.Context, params *module.Parameters) error {
	for _, addr := range strings.Split(*m.quakeServers, ",") {
		node := network.MustMakeNode(params.Network)
		p := New(&Config{
			Address:     addr,
			IdleTimeout: clientTimeout,
		}, node)
		go p.Run(ctx)
	}
	return nil
}
