// +build nopcap

package phys

func openPcapHandle(f *Flags, captureNonIPX bool) (DuplexEthernetStream, error) {
	return nil, nil
}

func maybeAddPcapDeviceFlag(f *Flags) {
}

