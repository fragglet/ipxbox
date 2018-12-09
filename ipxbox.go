// Package main implements a standalone DOSbox-IPX server.
package main

import (
	"fmt"
	"log"

	"github.com/fragglet/ipxbox/server"
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
			if (i + 1) % 16 == 0 {
				fmt.Printf("\n")
			}
		}
		fmt.Printf("\n")
	}
}

func main() {
	s, err := server.New(":10000", server.DefaultConfig)
	if err != nil {
		log.Fatal(err)
	}

	//go printPackets(s)
	s.Run()
}
