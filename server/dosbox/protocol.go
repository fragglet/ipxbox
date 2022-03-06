// Package dosbox implements the server side of the DOSBox IPX protocol.
package dosbox

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/network/stats"
	"github.com/fragglet/ipxbox/server"
)

var (
	_ = (server.Protocol)(&Protocol{})
	_ = (ipx.ReadWriteCloser)(&Client{})

	// Server-initiated pings come from this address.
	addrPingReply = [6]byte{0x02, 0xff, 0xff, 0xff, 0x00, 0x00}
)

type Protocol struct {
	// A new Node is created in this network each time a new client
	// is created.
	Network network.Network

	// Always send at least one packet every few seconds to keep the
	// UDP connection open. Some NAT networks and firewalls can be very
	// aggressive about closing off the ability for clients to receive
	// packets on particular ports if nothing is received for a while.
	// This controls the time for keepalives.
	KeepaliveTime time.Duration

	// If not nil, log entries are written as clients connect and
	// disconnect.
	Logger *log.Logger
}

func (p *Protocol) log(format string, args ...interface{}) {
	if p.Logger != nil {
		p.Logger.Printf(format, args...)
	}
}

// StartClient is invoked as a new goroutine when a new client connects.
func (p *Protocol) StartClient(ctx context.Context, inner ipx.ReadWriteCloser, remoteAddr net.Addr) error {
	packet, err := inner.ReadPacket(ctx)
	if err != nil {
		return err
	}
	if !packet.Header.IsRegistrationPacket() {
		return nil
	}
	node := p.Network.NewNode()
	nodeAddr := network.NodeAddress(node)
	defer func() {
		node.Close()
		statsString := stats.Summary(node)
		if statsString != "" {
			p.log("%s (IPX address %s): final statistics: %s",
				remoteAddr.String(), nodeAddr.String(), statsString)
		}
	}()

	p.log("%s: new connection, assigned IPX address %s",
		remoteAddr.String(), network.NodeAddress(node))
	c := &Client{
		inner:        inner,
		nodeAddr:     &nodeAddr,
		lastRecvTime: time.Now(),
	}

	c.SendRegistrationReply()

	go c.SendKeepalives(ctx, p.KeepaliveTime)

	return ipx.DuplexCopyPackets(ctx, c, node)
}

// Client implements the dosbox protocol as a wrapper around an
// inner ReadWriteCloser that is used to send and receive IPX frames.
type Client struct {
	inner        ipx.ReadWriteCloser
	nodeAddr     *ipx.Addr
	mu           sync.Mutex
	lastRecvTime time.Time
}

func (p *Client) ReadPacket(ctx context.Context) (*ipx.Packet, error) {
	for {
		packet, err := p.inner.ReadPacket(ctx)
		if err != nil {
			return nil, err
		}
		p.mu.Lock()
		p.lastRecvTime = time.Now()
		p.mu.Unlock()
		if packet.Header.IsRegistrationPacket() {
			p.SendRegistrationReply()
			continue
		}
		return packet, nil
	}
}

func (p *Client) WritePacket(packet *ipx.Packet) error {
	return p.inner.WritePacket(packet)
}

func (p *Client) Close() error {
	return p.inner.Close()
}

// SendRegistrationReply sends a response to the client when a registration
// packet is received. This usually happens only once on first connect,
// unless the reply is lost in transit.
func (p *Client) SendRegistrationReply() {
	p.inner.WritePacket(&ipx.Packet{
		Header: ipx.Header{
			Checksum:     0xffff,
			Length:       30,
			TransControl: 0,
			Dest: ipx.HeaderAddr{
				Network: [4]byte{0, 0, 0, 0},
				Addr:    *p.nodeAddr,
				Socket:  2,
			},
			Src: ipx.HeaderAddr{
				Network: [4]byte{0, 0, 0, 1},
				Addr:    ipx.AddrBroadcast,
				Socket:  2,
			},
		},
	})
}

// sendPing transmits a ping packet to the given client. The DOSbox IPX client
// code recognizes broadcast packets sent to socket=2 and will send a reply to
// the source address that we provide.
func (p *Client) sendPing() {
	p.inner.WritePacket(&ipx.Packet{
		Header: ipx.Header{
			Dest: ipx.HeaderAddr{
				Addr:   ipx.AddrBroadcast,
				Socket: 2,
			},
			// We send pings from an imaginary "ping reply" address
			// because if we used ipx.AddrNull the reply would be
			// indistinguishable from a registration packet.
			Src: ipx.HeaderAddr{
				Addr:   addrPingReply,
				Socket: 0,
			},
		},
	})
}

// SendKeepalives runs as a background goroutine while a client is connected,
// sending keepalive pings to keep the connection alive.
func (p *Client) SendKeepalives(ctx context.Context, checkPeriod time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(checkPeriod):
		}
		now := time.Now()
		p.mu.Lock()
		lastRecvTime := p.lastRecvTime
		p.mu.Unlock()
		// Nothing sent in a while? Send a keepalive. This is
		// important because some games use a client/server
		// arrangement where the server does not broadcast
		// anything but listens for broadcasts from clients. An
		// example is Warcraft 2. If there is no activity
		// between the client and server in a long time, some
		// NAT gateways or firewalls can drop the association.
		if now.After(lastRecvTime.Add(checkPeriod)) {
			p.sendPing()
		}
	}
}
