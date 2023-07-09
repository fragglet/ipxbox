// +build nopcap

package phys

import (
	"fmt"
)

func openPcapHandle(f *Flags, captureNonIPX bool) (DuplexEthernetStream, error) {
	return nil, fmt.Errorf("libpcap support not compiled in")
}

func maybeAddPcapDeviceFlag(f *Flags) {
}

