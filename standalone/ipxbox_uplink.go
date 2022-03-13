// Package main is a standalone program that will connect to an ipxbox uplink
// server and bridge to a local physical network.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/fragglet/ipxbox/client/uplink"
	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network/filter"
	"github.com/fragglet/ipxbox/phys"
)

var (
	uplinkServer = flag.String("uplink_server", "", "Address of IPX uplink server.")
	password     = flag.String("password", "", "Password for uplink server.")
	allowNetBIOS = flag.Bool("allow_netbios", false, "If true, allow packets to be forwarded that may contain Windows file sharing (NetBIOS) packets.")
)

func main() {
	physFlags := phys.RegisterFlags()
	flag.Parse()
	if *uplinkServer == "" || *password == "" {
		log.Fatalf("Uplink server and/or password no specified. Please specify --uplink_server and --password.")
	}
	ctx := context.Background()
	physLink, err := physFlags.MakePhys(false)
	if err != nil {
		log.Fatalf("failed to open physical network: %v", err)
	}
	if physLink == nil {
		log.Fatalf("No physical network specified. Please specify --pcap_device.")
	}

	conn, err := uplink.Dial(ctx, *uplinkServer, *password)
	if err != nil {
		log.Fatalf("failed to connect to server: %v", err)
	}
	defer conn.Close()
	go physLink.Run()
	if !*allowNetBIOS {
		conn = filter.New(conn)
	}
	if err := ipx.DuplexCopyPackets(ctx, conn, physLink); err != nil {
		log.Fatalf("error while copying packets: %v", err)
	}
}
