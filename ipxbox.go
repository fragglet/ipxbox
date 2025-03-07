// Package main implements a standalone DOSbox-IPX server.
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/module/aggregate"
	"github.com/fragglet/ipxbox/module/bridge"
	"github.com/fragglet/ipxbox/module/ipxpkt"
	"github.com/fragglet/ipxbox/module/pptp"
	"github.com/fragglet/ipxbox/module/qproxy"
	"github.com/fragglet/ipxbox/module/server"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/network/addressable"
	"github.com/fragglet/ipxbox/network/filter"
	"github.com/fragglet/ipxbox/network/ipxswitch"
	"github.com/fragglet/ipxbox/network/stats"
	"github.com/fragglet/ipxbox/network/tappable"
	"github.com/fragglet/ipxbox/phys"
	"github.com/fragglet/ipxbox/syslog"

	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
)

var (
	dumpPackets  = flag.String("dump_packets", "", "Write packets to a .pcap file with the given name.")
	allowNetBIOS = flag.Bool("allow_netbios", false, "If true, allow packets to be forwarded that may contain Windows file sharing (NetBIOS) packets.")
	enableSyslog = flag.Bool("enable_syslog", false, "If true, client connects/disconnects are logged to syslog")
	enableIpxpkt = flag.Bool("enable_ipxpkt", false, "If true, route encapsulated packets from the IPXPKT.COM driver to the physical network")
	enablePPTP   = flag.Bool("enable_pptp", false, "If true, run PPTP VPN server on TCP port 1723.")
)

func makePcapWriter() *pcapgo.Writer {
	var f *os.File
	// eg. `ipxbox --dump_packets /dev/stdout | tcpdump -nlr -`
	if *dumpPackets == "-" {
		f = os.Stdout
	} else {
		var err error
		f, err = os.Create(*dumpPackets)
		if err != nil {
			log.Fatalf("failed to open pcap file for write: %v", err)
		}
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
	mainmod := aggregate.MakeModule(
		module.Optional(ipxpkt.Module, enableIpxpkt),
		module.Optional(pptp.Module, enablePPTP),
		bridge.Module,
		qproxy.Module,
		server.Module,
	)

	mainmod.Initialize()

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

	err := mainmod.Start(ctx, &module.Parameters{
		Network:    net,
		Uplinkable: uplinkable,
		Logger:     logger,
	})
	if err != nil {
		log.Fatalf("server terminated with error: %v", err)
	}
}
