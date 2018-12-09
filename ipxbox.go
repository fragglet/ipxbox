// Package main implements a standalone DOSbox-IPX server.
package main

import (
	"log"

	"github.com/fragglet/ipxbox/server"
)

func main() {
	s := server.New(server.DefaultConfig)
	if err := s.Listen(":10000"); err != nil {
		log.Fatal(err)
	}

	for {
		if err := s.Poll(); err != nil {
			log.Fatal(err)
		}
	}
}
