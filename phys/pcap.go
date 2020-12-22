// Package phys implements a physical packet interface that uses libpcap
// to send and receive packets on a physical network interface.
package phys

import (
	"github.com/google/gopacket/pcap"
)

func NewPcap(handle *pcap.Handle, framer Framer) (*Phys, error) {
	if err := handle.SetBPFFilter("ipx"); err != nil {
		return nil, err
	}
	return newPhys(handle, framer), nil
}
