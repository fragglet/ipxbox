// Package dosbox implements a client for connecting to a DOSbox IPX server
// over UDP.
package dosbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	udpclient "github.com/fragglet/ipxbox/client"
	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/network/pipe"
)

const maxConnectAttempts = 5

var (
	_ = (network.Node)(&client{})
)

type connectFailure struct {
	addr string
}

func (cf *connectFailure) Error() string {
	return fmt.Sprintf("failed to connect to server %q", cf.addr)
}

func (cf *connectFailure) Unwrap() error {
	return os.ErrDeadlineExceeded
}

type client struct {
	inner  ipx.ReadWriteCloser
	rxpipe ipx.ReadWriteCloser
	addr   ipx.Addr
}

func (c *client) ReadPacket(ctx context.Context) (*ipx.Packet, error) {
	return c.rxpipe.ReadPacket(ctx)
}

func (c *client) WritePacket(packet *ipx.Packet) error {
	return c.inner.WritePacket(packet)
}

func (c *client) Close() error {
	c.rxpipe.Close()
	return c.inner.Close()
}

func (c *client) GetProperty(x interface{}) bool {
	switch x.(type) {
	case *ipx.Addr:
		*x.(*ipx.Addr) = c.addr
		return true
	default:
		return false
	}
}

func (c *client) sendPingReply(addr *ipx.Addr) {
	c.inner.WritePacket(&ipx.Packet{
		Header: ipx.Header{
			Dest: ipx.HeaderAddr{
				Addr:   *addr,
				Socket: 2,
			},
			Src: ipx.HeaderAddr{
				Addr:   c.addr,
				Socket: 0,
			},
		},
	})
}

func isPing(hdr *ipx.Header) bool {
	return hdr.Dest.Addr == ipx.AddrBroadcast && hdr.Dest.Socket == 2
}

func (c *client) recvLoop(ctx context.Context) {
	for {
		packet, err := c.inner.ReadPacket(ctx)
		if errors.Is(err, io.ErrClosedPipe) {
			break
		} else if err != nil {
			// TODO: Log error?
			continue
		}

		// Respond to pings to keep the connection alive. Even if
		// ReadPacket() isn't being called regularly, we still respond
		// to pings to ensure the connection stays alive. For the same
		// reason the pinger will always get a decent RTT measurement.
		if isPing(&packet.Header) {
			c.sendPingReply(&packet.Header.Src.Addr)
			continue
		}

		c.rxpipe.WritePacket(packet)
	}
}

func sendRegistrationPacket(c ipx.ReadWriteCloser) {
	c.WritePacket(&ipx.Packet{
		Header: ipx.Header{
			Dest: ipx.HeaderAddr{
				Addr:   ipx.AddrNull,
				Socket: 2,
			},
			Src: ipx.HeaderAddr{
				Addr:   ipx.AddrNull,
				Socket: 2,
			},
		},
	})
}

func isRegistrationResponse(hdr *ipx.Header) bool {
	return hdr.Dest.Socket == 2 && hdr.Src.Socket == 2 && hdr.Dest.Addr != ipx.AddrBroadcast
}

func handshakeConnect(ctx context.Context, c ipx.ReadWriteCloser, addr string) (ipx.Addr, error) {
	nextSendTime := time.Now()
	connectAttempts := 0
	for {
		now := time.Now()
		if now.After(nextSendTime) {
			if connectAttempts >= maxConnectAttempts {
				return ipx.AddrNull, &connectFailure{addr}
			}
			sendRegistrationPacket(c)
			connectAttempts++
			nextSendTime = now.Add(time.Second)
		}
		subctx, _ := context.WithDeadline(ctx, nextSendTime)
		packet, err := c.ReadPacket(subctx)
		if errors.Is(err, context.DeadlineExceeded) {
			continue
		}
		if err != nil {
			return ipx.AddrNull, err
		}
		if isRegistrationResponse(&packet.Header) {
			return packet.Header.Dest.Addr, nil
		}
	}
}

func Dial(ctx context.Context, addr string) (network.Node, error) {
	udp, err := udpclient.Dial(addr)
	if err != nil {
		return nil, err
	}
	c := &client{
		inner:  udp,
		rxpipe: pipe.New(1),
	}
	if c.addr, err = handshakeConnect(ctx, udp, addr); err != nil {
		return nil, err
	}
	go c.recvLoop(context.Background())
	return c, nil
}
