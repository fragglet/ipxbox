// Package main is a standalone program that will connect to an ipxbox uplink
// server and bridge to a local physical network.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/fragglet/ipxbox/client/uplink"
	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/module/bridge"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/network/filter"
)

var (
	uplinkServer = flag.String("uplink_server", "", "Address of IPX uplink server.")
	password     = flag.String("password", "", "Password for uplink server.")
	allowNetBIOS = flag.Bool("allow_netbios", false, "If true, allow packets to be forwarded that may contain Windows file sharing (NetBIOS) packets.")
)

type fakeNetwork struct {
	ipx.ReadWriteCloser
}

func (f *fakeNetwork) NewNode() (network.Node, error) {
	return f, nil
}

func (f *fakeNetwork) GetProperty(value interface{}) bool {
	return false
}

func main() {
	ctx := context.Background()
	mod := bridge.Module
	mod.Initialize()
	flag.Parse()
	if *uplinkServer == "" || *password == "" {
		log.Fatalf("Uplink server and/or password no specified. Please specify --uplink_server and --password.")
	}
	conn, err := uplink.Dial(ctx, *uplinkServer, *password)
	if err != nil {
		log.Fatalf("failed to connect to server: %v", err)
	}
	defer conn.Close()
	if !*allowNetBIOS {
		conn = filter.New(conn)
	}
	err = mod.Start(ctx, &module.Parameters{
		Uplinkable: &fakeNetwork{conn},
	})
	if err != nil {
		log.Fatalf("bridge exited with error: %v", err)
	}
}
