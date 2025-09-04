// Package main implements a standalone DOSbox-IPX server. This is a
// minimal version that is *just* an IPX server.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/fragglet/ipxbox/logging"
	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/module/server"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/network/addressable"
	"github.com/fragglet/ipxbox/network/filter"
	"github.com/fragglet/ipxbox/network/ipxswitch"
	"github.com/fragglet/ipxbox/network/stats"
)

var (
	allowNetBIOS = flag.Bool("allow_netbios", false, "If true, allow packets to be forwarded that may contain Windows file sharing (NetBIOS) packets.")
)

func makeNetwork(ctx context.Context) (network.Network, network.Network) {
	var net network.Network
	net = ipxswitch.New()
	if !*allowNetBIOS {
		net = filter.Wrap(net)
	}
	uplinkable := net
	net = addressable.Wrap(net)
	net = stats.Wrap(net)
	return net, stats.Wrap(uplinkable)
}

func main() {
	mainmod := server.Module

	mainmod.Initialize()
	logspec := logging.RegisterFlag()

	flag.Parse()

	ctx := context.Background()

	logger, err := logspec.MakeLogger()
	if err != nil {
		log.Fatalf("error initializing logging: %v", err)
	}

	net, uplinkable := makeNetwork(ctx)

	err = mainmod.Start(ctx, &module.Parameters{
		Network:    net,
		Uplinkable: uplinkable,
		Logger:     logger,
	})
	if err != nil {
		log.Fatalf("server terminated with error: %v", err)
	}
}
