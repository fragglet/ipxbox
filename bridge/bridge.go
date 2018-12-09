// Package bridge implements an IPX bridge.
package bridge

import (
	"io"
	"sync"

	"github.com/fragglet/ipxbox/ipx"
)

func copyPackets(in io.ReadCloser, out io.WriteCloser) {
	localAddresses := map[ipx.Addr]bool{}
	for {
		buf := make([]byte, 1500)
		n, err := in.Read(buf)
		if err != nil {
			break
		}
		buf = buf[0:n]

		var hdr ipx.Header
		if err := hdr.UnmarshalBinary(buf); err != nil {
			continue
		}
		// Remember every address we see from the input device, and
		// don't copy packets if the destination is on the input device.
		localAddresses[hdr.Src.Addr] = true
		if localAddresses[hdr.Dest.Addr] {
			continue
		}
		out.Write(buf)
	}
	in.Close()
	out.Close()
}

// Run implements an IPX bridge, copying IPX packets from in1 to out2 and from
// in2 to out1. Copying will stop if an error occurs (eg. if one of the inputs
// is closed) and all the devices will be closed.
func Run(in1 io.ReadCloser, out1 io.WriteCloser, in2 io.ReadCloser, out2 io.WriteCloser) {
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
