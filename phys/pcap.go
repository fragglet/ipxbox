// +build !nopcap

package phys

import (
	"fmt"
	"strings"

	"github.com/google/gopacket/pcap"
)

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

func openPcapHandle(deviceName string, captureNonIPX bool) (DuplexEthernetStream, error) {
	if deviceName == "" {
		return nil, nil
	}
	if deviceName == "list" {
		devices, err := listNetDevices()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("valid network devices are: %v", devices)
	}
	handle, err := pcap.OpenLive(deviceName, 1500, true, pcap.BlockForever)
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
