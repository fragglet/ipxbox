// Package main implements a standalone DOSbox-IPX server.
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/fragglet/ipxbox/bridge"
	"github.com/fragglet/ipxbox/ipxpkt"
	"github.com/fragglet/ipxbox/phys"
	"github.com/fragglet/ipxbox/server"
	"github.com/fragglet/ipxbox/virtual"

	"github.com/google/gopacket/pcap"
	"github.com/songgao/water"
)

var framers = map[string]phys.Framer{
	"802.2":    phys.Framer802_2,
	"802.3raw": phys.Framer802_3Raw,
	"snap":     phys.FramerSNAP,
	"eth-ii":   phys.FramerEthernetII,
}

var (
	pcapDevice      = flag.String("pcap_device", "", `Send and receive packets to the given device ("list" to list all devices)`)
	enableTap       = flag.Bool("enable_tap", false, "Bridge the server to a tap device.")
	dumpPackets     = flag.Bool("dump_packets", false, "Dump packets to stdout.")
	port            = flag.Int("port", 10000, "UDP port to listen on.")
	clientTimeout   = flag.Duration("client_timeout", server.DefaultConfig.ClientTimeout, "Time of inactivity before disconnecting clients.")
	ethernetFraming = flag.String("ethernet_framing", "802.2", `Framing to use when sending Ethernet packets. Valid values are "802.2", "802.3raw", "snap" and "eth-ii".`)
	allowNetBIOS    = flag.Bool("allow_netbios", false, "If true, allow packets to be forwarded that may contain Windows file sharing (NetBIOS) packets.")
	enableIpxpkt    = flag.Bool("enable_ipxpkt", false, "If true, route encapsulated packets from the IPXPKT.COM driver to the physical network (requires --enable_tap or --pcap_device)")
)

func printPackets(v *virtual.Network) {
	tap := v.Tap()
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

func ethernetStream() (phys.DuplexEthernetStream, error) {
	if *enableTap {
		return phys.NewTap(water.Config{})
	} else if *pcapDevice == "" {
		return nil, nil
	}
	// TODO: List
	handle, err := pcap.OpenLive(*pcapDevice, 1500, true, pcap.BlockForever)
	if err != nil {
		return nil, err
	}
	// As an optimization we set a filter to only deliver IPX packets
	// because they're all we care about. However, when ipxpkt routing is
	// enabled we want all Ethernet frames.
	if !*enableIpxpkt {
		if err := handle.SetBPFFilter("ipx"); err != nil {
			return nil, err
		}
	}
	return handle, nil
}

func main() {
	flag.Parse()

	var cfg server.Config
	cfg = *server.DefaultConfig
	cfg.ClientTimeout = *clientTimeout
	v := virtual.New()
	v.BlockNetBIOS = !*allowNetBIOS

	stream, err := ethernetStream()
	if err != nil {
		log.Fatalf("failed to set up physical network: %v", err)
	} else if stream != nil {
		framer, ok := framers[*ethernetFraming]
		if !ok {
			log.Fatalf("unknown Ethernet framing %q", *ethernetFraming)
		}

		p := phys.NewPhys(stream, framer)
		tap := v.Tap()
		go bridge.Run(tap, tap, p, p)
		if *enableIpxpkt {
			r := ipxpkt.NewRouter(v.NewNode())
			go phys.CopyFrames(r, p.NonIPX())
		}
	}
	if *dumpPackets {
		go printPackets(v)
	}

	s, err := server.New(fmt.Sprintf(":%d", *port), v, &cfg)
	if err != nil {
		log.Fatal(err)
	}
	s.Run()
}
