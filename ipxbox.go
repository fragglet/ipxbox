// Package main implements a standalone DOSbox-IPX server.
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/fragglet/ipxbox/bridge"
	"github.com/fragglet/ipxbox/phys"
	"github.com/fragglet/ipxbox/server"
	"github.com/songgao/water"
)

var (
	enableTap   = flag.Bool("enable_tap", false, "Bridge the server to a tap device.")
	dumpPackets = flag.Bool("dump_packets", false, "Dump packets to stdout.")
	port        = flag.Int("port", 10000, "UDP port to listen on.")
)

func printPackets(s *server.Server) {
	tap := s.Tap()
	defer tap.Close()
	for {
		buf := make([]byte, 1500)
		n, err := tap.Read(buf)
		if err != nil {
			break
		}
		fmt.Printf("packet:\n")
		for i := 0; i < n; i++ {
			fmt.Printf("%02x ", buf[i])
			if (i+1)%16 == 0 {
				fmt.Printf("\n")
			}
		}
		fmt.Printf("\n")
	}
}

func main() {
	flag.Parse()
	s, err := server.New(fmt.Sprintf(":%d", *port), server.DefaultConfig)
	if err != nil {
		log.Fatal(err)
	}
	switch {
	case *enableTap:
		p, err := phys.New(water.Config{})
		if err != nil {
			log.Fatalf("failed to start tap: %v", err)
		}
		go bridge.Run(s.Tap(), s, p, p)
	case *dumpPackets:
		go printPackets(s)
	}
	s.Run()
}
