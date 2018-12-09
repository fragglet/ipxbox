// Package main implements an IPX DOSbox-physical bridge.
package main

import (
	"log"

	"github.com/fragglet/ipxbox/bridge"
	"github.com/fragglet/ipxbox/phys"
	"github.com/fragglet/ipxbox/server"
	"github.com/songgao/water"
)

func main() {
	p, err := phys.New(water.Config{})
	if err != nil {
		log.Fatalf("failed to start tap: %v", err)
	}
	s, err := server.New(":10000", server.DefaultConfig)
	if err != nil {
		log.Fatal(err)
	}
	go s.Run()
	bridge.Run(s.Tap(), s, p, p)
}
