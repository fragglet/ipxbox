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
		devices, err := listNetDevices()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("valid pcap network devices are: %v", devices)
	}
	handle, err := pcap.OpenLive(deviceName, 1500, true, pcap.BlockForever)
	if err != nil {
		return nil, err
	}
	// Only deliver received packets, otherwise packets *we* inject into
	// the network will get delivered back to us.
	handle.SetDirection(pcap.DirectionIn)

	filter := "ipx"
	if captureNonIPX {
		filter = "not ipx"
	}
	if err := handle.SetBPFFilter(filter); err != nil {
		return nil, err
	}
	return handle, nil
}

func init() {
	types["pcap"] = openPcapHandle
}
