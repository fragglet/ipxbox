package smb

import (
	"context"
	"flag"
	"fmt"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/network"
)

type mod struct {
	name string
}

var (
	Module = (module.Module)(&mod{})
)

func (m *mod) Initialize() {
	flag.StringVar(&m.name, "smb_name", "IPXBOX", "NetBIOS name to announce to the network.")
}

func (m *mod) Start(ctx context.Context, params *module.Parameters) error {
	node, err := params.Network.NewNode()
	if err != nil {
		return err
	}
	addr := ipx.HeaderAddr{
		Addr:   network.NodeAddress(node),
		Socket: nmpiSocket,
	}
	a := NewNmpiAnnouncer(m.name, 0x20, addr, node)
	if err = a.Register(); err != nil {
		return fmt.Errorf("failed to register name %q as NetBIOS name: %v", m.name, err)
	}

	for {
		pkt, err := node.ReadPacket(ctx)
		if err != nil {
			return err
		}
		switch pkt.Header.Dest.Socket {
		case nmpiSocket:
			a.WritePacket(pkt)
			// TODO: Handle other sockets
		}
	}

	return nil
}
