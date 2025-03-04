// +build nopcap

package phys

func openPcapHandle(deviceName string, captureNonIPX bool) (DuplexEthernetStream, error) {
	return nil, nil
}
