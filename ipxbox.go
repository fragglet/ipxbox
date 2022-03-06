// Package main implements a standalone DOSbox-IPX server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/ipxpkt"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/network/addressable"
	"github.com/fragglet/ipxbox/network/filter"
	"github.com/fragglet/ipxbox/network/ipxswitch"
	"github.com/fragglet/ipxbox/network/stats"
	"github.com/fragglet/ipxbox/network/tappable"
	"github.com/fragglet/ipxbox/phys"
	"github.com/fragglet/ipxbox/ppp/pptp"
	"github.com/fragglet/ipxbox/qproxy"
	"github.com/fragglet/ipxbox/server"
	"github.com/fragglet/ipxbox/server/dosbox"
	"github.com/fragglet/ipxbox/syslog"

	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"
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
	dumpPackets     = flag.String("dump_packets", "", "Write packets to a .pcap file with the given name.")
	port            = flag.Int("port", 10000, "UDP port to listen on.")
	clientTimeout   = flag.Duration("client_timeout", 10*time.Minute, "Time of inactivity before disconnecting clients.")
	ethernetFraming = flag.String("ethernet_framing", "802.2", `Framing to use when sending Ethernet packets. Valid values are "802.2", "802.3raw", "snap" and "eth-ii".`)
	allowNetBIOS    = flag.Bool("allow_netbios", false, "If true, allow packets to be forwarded that may contain Windows file sharing (NetBIOS) packets.")
	enableIpxpkt    = flag.Bool("enable_ipxpkt", false, "If true, route encapsulated packets from the IPXPKT.COM driver to the physical network (requires --enable_tap or --pcap_device)")
	enableSyslog    = flag.Bool("enable_syslog", false, "If true, client connects/disconnects are logged to syslog")
	quakeServers    = flag.String("quake_servers", "", "Proxy to the given list of Quake UDP servers in a way that makes them accessible over IPX.")
	enablePPTP      = flag.Bool("enable_pptp", false, "If true, run PPTP VPN server on TCP port 1723.")
)

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

func addQuakeProxies(ctx context.Context, net network.Network) {
	if *quakeServers == "" {
		return
	}
	for _, addr := range strings.Split(*quakeServers, ",") {
		p := qproxy.New(&qproxy.Config{
			Address:     addr,
			IdleTimeout: *clientTimeout,
		}, net.NewNode())
		go p.Run(ctx)
	}
}

func makePcapWriter() *pcapgo.Writer {
	f, err := os.Create(*dumpPackets)
	if err != nil {
		log.Fatalf("failed to open pcap file for write: %v", err)
	}
	w := pcapgo.NewWriter(f)
	w.WriteFileHeader(1500, layers.LinkTypeEthernet)
	return w
}

func makeNetwork(ctx context.Context) (network.Network, network.Network) {
	// We build the network up in layers, each layer adding an extra
	// feature. This approach allows for modularity and separation of
	// concerns, avoiding the complexity of a big monolithic system.
	// This is best read in reverse order. Life of an rx packet:
	//  1. Packet received from client; WritePacket() by server
	//  2. Check source address matches client address (addressable)
	//  3. Increment receive statistics (stats)
	//  4. Drop packet if a NetBIOS packet (filter)
	//  5. Fork incoming traffic to any network taps (tappable)
	//  6. Forward to receive queue(s) of other clients (ipxswitch)
	// Then back out the other way (tx):
	//  1. Read packet from receive queue (ipxswitch)
	//  2. No-op (tappable)
	//  3. Filter NetBIOS packets (filter)
	//  4. Increment transmit statistics (stats)
	//  5. Check dest address matches client address (addressable)
	//  5. ReadPacket() by server, and transmit to client.
	var net network.Network
	net = ipxswitch.New()
	if *dumpPackets != "" {
		tappableLayer := tappable.Wrap(net)
		w := makePcapWriter()
		sink := phys.NewPcapgoSink(w, phys.FramerEthernetII)
		go ipx.CopyPackets(ctx, tappableLayer.NewTap(), sink)
		net = tappableLayer
	}
	if !*allowNetBIOS {
		net = filter.Wrap(net)
	}
	uplinkable := net
	net = addressable.Wrap(net)
	net = stats.Wrap(net)
	return net, uplinkable
}

func main() {
	flag.Parse()

	ctx := context.Background()

	var logger *log.Logger
	if *enableSyslog {
		var err error
		logger, err = syslog.NewLogger(
			syslog.LOG_NOTICE|syslog.LOG_DAEMON, 0)
		if err != nil {
			log.Fatalf("failed to init syslog: %v", err)
		}
	}

	net, uplinkable := makeNetwork(ctx)

	stream, err := ethernetStream()
	if err != nil {
		log.Fatalf("failed to set up physical network: %v", err)
	} else if stream != nil {
		framer, ok := framers[*ethernetFraming]
		if !ok {
			log.Fatalf("unknown Ethernet framing %q", *ethernetFraming)
		}

		p := phys.NewPhys(stream, framer)
		port := uplinkable.NewNode()
		go p.Run()
		go ipx.DuplexCopyPackets(ctx, p, port)
		if *enableIpxpkt {
			r := ipxpkt.NewRouter(net.NewNode())
			go phys.CopyFrames(r, p.NonIPX())
		}
	}
	addQuakeProxies(ctx, net)
	if *enablePPTP {
		pptps, err := pptp.NewServer(net)
		if err != nil {
			log.Fatalf("failed to start PPTP server: %v", err)
		}
		go pptps.Run(ctx)
	}

	s, err := server.New(fmt.Sprintf(":%d", *port), &server.Config{
		Protocol: &dosbox.Protocol{
			Logger:        logger,
			Network:       net,
			KeepaliveTime: 5 * time.Second,
		},
		ClientTimeout: *clientTimeout,
		Logger:        logger,
	})
	if err != nil {
		log.Fatal(err)
	}
	s.Run(ctx)
}
