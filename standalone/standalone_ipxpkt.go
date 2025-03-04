// Package main implements a standalone proxy that connects to a DOSbox
// server and provides an ipxpkt bridge.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/fragglet/ipxbox/client/dosbox"
	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/module/ipxpkt"
)

var (
	dosboxServer = flag.String("dosbox_server", "", "Address of DOSbox IPX server.")
)

func main() {
	ctx := context.Background()

	mod := ipxpkt.Module
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
