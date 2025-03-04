// Package main implements a standalone proxy that connects to a DOSbox
// server and translates IPX packets into UDP packets that are forwarded
// to a UDP Quake server.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/fragglet/ipxbox/client/dosbox"
	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/module/qproxy"
)

var (
	dosboxServer = flag.String("dosbox_server", "", "Address of DOSbox IPX server.")
)

func main() {
	ctx := context.Background()

	mod := qproxy.Module
	mod.Initialize()
	flag.Parse()

	if *dosboxServer == "" {
		log.Fatalf("no address given for -dosbox_server")
	}

	err := mod.Start(ctx, &module.Parameters{
		Network: &dosbox.Client{ctx, *dosboxServer},
	})
	if err != nil {
		log.Fatalf("server terminated with error: %v", err)
	}
}
