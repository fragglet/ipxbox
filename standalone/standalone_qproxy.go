// Package main implements a standalone proxy that connects to a DOSbox
// server and translates IPX packets into UDP packets that are forwarded
// to a UDP Quake server.
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/fragglet/ipxbox/client/dosbox"
	"github.com/fragglet/ipxbox/qproxy"
)

var (
	dosboxServer = flag.String("dosbox_server", "", "Address of DOSbox IPX server.")
	quakeServer  = flag.String("quake_server", "", "Address of Quake server.")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	node, err := dosbox.Dial(ctx, *dosboxServer)
	if err != nil {
		log.Fatalf("failed to connect to server: %v", err)
	}

	config := &qproxy.Config{
		Address:     *quakeServer,
		IdleTimeout: 60 * time.Second,
	}

	proxy := qproxy.New(config, node)
	proxy.Run(ctx)
}
