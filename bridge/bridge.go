// Package bridge implements an IPX bridge.
package bridge

import (
	"sync"

	"github.com/fragglet/ipxbox/ipx"
)

func copyPackets(in ipx.ReadCloser, out ipx.WriteCloser) {
	localAddresses := map[ipx.Addr]bool{}
	for {
		packet, err := in.ReadPacket()
		if err != nil {
			break
		}

		// Remember every address we see from the input device, and
		// don't copy packets if the destination is on the input device.
		localAddresses[packet.Header.Src.Addr] = true
		if localAddresses[packet.Header.Dest.Addr] {
			continue
		}
		out.WritePacket(packet)
	}
	in.Close()
	out.Close()
}

// Run implements an IPX bridge, copying IPX packets from in1 to out2 and from
// in2 to out1. Copying will stop if an error occurs (eg. if one of the inputs
// is closed) and all the devices will be closed.
func Run(in1 ipx.ReadCloser, out1 ipx.WriteCloser, in2 ipx.ReadCloser, out2 ipx.WriteCloser) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		copyPackets(in1, out2)
		in2.Close()
		wg.Done()
	}()
	go func() {
		copyPackets(in2, out1)
		in1.Close()
		wg.Done()
	}()
	wg.Wait()
}
