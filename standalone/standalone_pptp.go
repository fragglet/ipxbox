// Package main implements a standalone PPTP server, passing through all
// connections to a remote DOSbox server.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/fragglet/ipxbox/client/dosbox"
	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/module/pptp"
)

var (
	dosboxServer = flag.String("dosbox_server", "", "Address of DOSbox IPX server.")
)

func main() {
	ctx := context.Background()

	mod := pptp.Module
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
