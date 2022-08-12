package phys

import (
	"flag"
	"fmt"
	"github.com/google/gopacket/pcap"
	"github.com/songgao/water"
	"strings"
)

type Flags struct {
	PcapDevice      *string
	EnableTap       *bool
	EthernetFraming *string
}

func RegisterFlags() *Flags {
	f := &Flags{}
	f.PcapDevice = flag.String("pcap_device", "", `Send and receive packets to the given device ("list" to list all devices)`)
	f.EnableTap = flag.Bool("enable_tap", false, "Bridge the server to a tap device.")
	f.EthernetFraming = flag.String("ethernet_framing", "auto", `Framing to use when sending Ethernet packets. Valid values are "auto", "802.2", "802.3raw", "snap" and "eth-ii".`)
	return f
}

func listNetDevices() (string, error) {
	ifaces, err := pcap.FindAllDevs()
	if err != nil {
		return "", err
	}
	result := []string{}
	for _, iface := range ifaces {
		result = append(result, fmt.Sprintf("%q", iface.Name))
	}
	return strings.Join(result, ", "), nil
}

func (f *Flags) EthernetStream(captureNonIPX bool) (DuplexEthernetStream, error) {
	if *f.EnableTap {
		return NewTap(water.Config{})
	} else if *f.PcapDevice == "" {
		return nil, nil
	}
	if *f.PcapDevice == "list" {
		devices, err := listNetDevices()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("valid network devices are: %v", devices)
	}
	handle, err := pcap.OpenLive(*f.PcapDevice, 1500, true, pcap.BlockForever)
	if err != nil {
		return nil, err
	}
	// Only deliver received packets, otherwise packets *we* inject into
	// the network will get delivered back to us.
	handle.SetDirection(pcap.DirectionIn)
	// As an optimization we set a filter to only deliver IPX packets
	// because they're all we care about. However, when ipxpkt routing is
	// enabled we want all Ethernet frames.
	if !captureNonIPX {
		if err := handle.SetBPFFilter("ipx"); err != nil {
			return nil, err
		}
	}
	return handle, nil
}

func (f *Flags) makeFramer() (Framer, error) {
	framerName := *f.EthernetFraming
	if framerName == "auto" {
		return &automaticFramer{
			fallback: Framer802_2,
		}, nil
	}
	for _, framer := range allFramers {
		if framerName == framer.Name() {
			return framer, nil
		}
	}
	return nil, fmt.Errorf("unknown Ethernet framing %q", framerName)
}

func (f *Flags) MakePhys(captureNonIPX bool) (*Phys, error) {
	stream, err := f.EthernetStream(captureNonIPX)
	if err != nil {
		return nil, err
	} else if stream != nil {
		framer, err := f.makeFramer()
		if err != nil {
			return nil, err
		}
		return NewPhys(stream, framer), nil
	}
	// Physical capture not enabled.
	return nil, nil
}
