package qproxy

import (
	"context"
	"flag"
	"strings"
	"time"

	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/qproxy"
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

func (m *mod) Start(ctx context.Context, net network.Network) {
	for _, addr := range strings.Split(*m.quakeServers, ",") {
		node := network.MustMakeNode(net)
		p := qproxy.New(&qproxy.Config{
			Address:     addr,
			IdleTimeout: clientTimeout,
		}, node)
		go p.Run(ctx)
	}
}
