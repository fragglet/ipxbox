// Package main implements a standalone DOSbox-IPX server.
package main

import (
	"log"

	"github.com/fragglet/ipxbox/server"
)

func main() {
	s, err := server.New(":10000", server.DefaultConfig)
	if err != nil {
		log.Fatal(err)
	}

	s.Run()
}
