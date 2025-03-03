// Package main implements a standalone DOSbox-IPX server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/ipxpkt"
	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/module/qproxy"
	"github.com/fragglet/ipxbox/module/pptp"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/network/addressable"
	"github.com/fragglet/ipxbox/network/filter"
	"github.com/fragglet/ipxbox/network/ipxswitch"
	"github.com/fragglet/ipxbox/network/stats"
	"github.com/fragglet/ipxbox/network/tappable"
	"github.com/fragglet/ipxbox/phys"
	"github.com/fragglet/ipxbox/server"
	"github.com/fragglet/ipxbox/server/dosbox"
	"github.com/fragglet/ipxbox/server/uplink"
	"github.com/fragglet/ipxbox/syslog"

	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
)

var (
	dumpPackets    = flag.String("dump_packets", "", "Write packets to a .pcap file with the given name.")
	port           = flag.Int("port", 10000, "UDP port to listen on.")
	clientTimeout  = flag.Duration("client_timeout", 10*time.Minute, "Time of inactivity before disconnecting clients.")
	allowNetBIOS   = flag.Bool("allow_netbios", false, "If true, allow packets to be forwarded that may contain Windows file sharing (NetBIOS) packets.")
	enableIpxpkt   = flag.Bool("enable_ipxpkt", false, "If true, route encapsulated packets from the IPXPKT.COM driver to the physical network (requires --enable_tap or --pcap_device)")
	enableSyslog   = flag.Bool("enable_syslog", false, "If true, client connects/disconnects are logged to syslog")
	uplinkPassword = flag.String("uplink_password", "", "Password to permit uplink clients to connect. If empty, uplink is not supported.")
)

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
	return net, stats.Wrap(uplinkable)
}

func main() {
	modules := []module.Module{
		qproxy.Module,
		pptp.Module,
	}

	for _, m := range modules {
		m.Initialize()
	}

	physFlags := phys.RegisterFlags()
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

	physLink, err := physFlags.MakePhys(*enableIpxpkt)
	if err != nil {
		log.Fatalf("failed to set up physical network: %v", err)
	} else if physLink != nil {
		port := network.MustMakeNode(uplinkable)
		go physLink.Run()
		go ipx.DuplexCopyPackets(ctx, physLink, port)
	}
	if *enableIpxpkt {
		port := network.MustMakeNode(net)
		r := ipxpkt.NewRouter(port)
		var tapConn phys.DuplexEthernetStream
		if physLink != nil {
			tapConn = physLink.NonIPX()
			log.Printf("Using physical network tap for ipxpkt router")
		} else {
			tapConn, err = phys.MakeSlirp()
			if err != nil {
				log.Fatalf("failed to open libslirp subprocess: %v.\nYou may need to install libslirp-helper, or alternatively use -enable_tap or -pcap_device.", err)
			}
			log.Printf("Using Slirp subprocess for ipxpkt router")
		}
		go phys.CopyFrames(r, tapConn)
	}

	for _, m := range modules {
		if m.IsEnabled() {
			m.Start(ctx, net)
		}
	}

	protocols := []server.Protocol{
		&dosbox.Protocol{
			Logger:        logger,
			Network:       net,
			KeepaliveTime: 5 * time.Second,
		},
	}
	if *uplinkPassword != "" {
		protocols = append(protocols, &uplink.Protocol{
			Logger:        logger,
			Network:       uplinkable,
			Password:      *uplinkPassword,
			KeepaliveTime: 5 * time.Second,
		})
	}
	s, err := server.New(fmt.Sprintf(":%d", *port), &server.Config{
		Protocols:     protocols,
		ClientTimeout: *clientTimeout,
		Logger:        logger,
	})
	if err != nil {
		log.Fatal(err)
	}
	s.Run(ctx)
}
