// Package bridge implements an IPX bridge.
package bridge

import (
	"context"
	"golang.org/x/sync/errgroup"

	"github.com/fragglet/ipxbox/ipx"
)

func copyPackets(ctx context.Context, in ipx.ReadCloser, out ipx.WriteCloser) error {
	localAddresses := map[ipx.Addr]bool{}
	for {
		packet, err := in.ReadPacket(ctx)
		if err != nil {
			return err
		}

		// Remember every address we see from the input device, and
		// don't copy packets if the destination is on the input device.
		localAddresses[packet.Header.Src.Addr] = true
		if localAddresses[packet.Header.Dest.Addr] {
			continue
		}
		out.WritePacket(packet)
	}
}

// Run implements an IPX bridge, copying IPX packets from in1 to out2 and from
// in2 to out1. Copying will stop if an error occurs (eg. if one of the inputs
// is closed) and all the devices will be closed.
func Run(ctx context.Context, in1 ipx.ReadCloser, out1 ipx.WriteCloser, in2 ipx.ReadCloser, out2 ipx.WriteCloser) {
	eg, egctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return copyPackets(egctx, in1, out2)
	})
	eg.Go(func() error {
		return copyPackets(egctx, in2, out1)
	})
	eg.Wait()
	in1.Close()
	out1.Close()
	in2.Close()
	out2.Close()
}
