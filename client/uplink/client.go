// Package uplink implements a client for connecting to an IPX uplink server
// over UDP.
package uplink

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	udpclient "github.com/fragglet/ipxbox/client"
	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network/pipe"
	"github.com/fragglet/ipxbox/server/uplink"
)

const maxConnectAttempts = 5

var (
	_ = (ipx.ReadWriteCloser)(&client{})
)

type client struct {
	inner  ipx.ReadWriteCloser
	rxpipe ipx.ReadWriteCloser
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

func (c *client) recvLoop(ctx context.Context) {
	for {
		packet, err := c.inner.ReadPacket(ctx)
		if errors.Is(err, io.ErrClosedPipe) {
			break
		} else if err != nil {
			// TODO: Log error?
			continue
		}
		if packet.Header.Dest.Addr == uplink.Address {
			continue
		}

		c.rxpipe.WritePacket(packet)
	}
}

func (c *client) sendUplinkMessage(msg *uplink.Message) error {
	jsonData, err := msg.Marshal()
	if err != nil {
		return err
	}
	return c.inner.WritePacket(&ipx.Packet{
		Header: ipx.Header{
			Dest: ipx.HeaderAddr{
				Addr: uplink.Address,
			},
		},
		Payload: jsonData,
	})
}

func (c *client) sendUntilResponse(ctx context.Context, msg *uplink.Message) (*uplink.Message, error) {
	nextSendTime := time.Now()
	connectAttempts := 0
	for {
		now := time.Now()
		if now.After(nextSendTime) {
			if connectAttempts >= maxConnectAttempts {
				return nil, fmt.Errorf("no response to %q message after %d attempts", msg.Type, connectAttempts)
			}
			c.sendUplinkMessage(msg)
			connectAttempts++
			nextSendTime = now.Add(time.Second)
		}
		subctx, _ := context.WithDeadline(ctx, nextSendTime)
		packet, err := c.inner.ReadPacket(subctx)
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			continue
		case err != nil:
			return nil, err
		case packet.Header.Dest.Addr != uplink.Address:
			continue
		}
		result := &uplink.Message{}
		if err := result.Unmarshal(packet.Payload); err != nil {
			return nil, err
		}
		return result, nil
	}
}

func (c *client) handshakeConnect(ctx context.Context, password string) error {
	clientChallenge := make([]byte, uplink.MinChallengeLength)
	if _, err := rand.Read(clientChallenge); err != nil {
		return err
	}
	clientSolution := uplink.SolveChallenge("server", password, clientChallenge)

	response, err := c.sendUntilResponse(ctx, &uplink.Message{
		Type: uplink.MessageTypeGetChallengeRequest,
	})
	switch {
	case err != nil:
		return err
	case response.Type != uplink.MessageTypeGetChallengeResponse:
		return fmt.Errorf("wrong response to challenge request: want %q, got %q", uplink.MessageTypeGetChallengeResponse, response.Type)
	case len(response.Challenge) < uplink.MinChallengeLength:
		return fmt.Errorf("server challenge too short: want minimum %d bytes, got %d", uplink.MinChallengeLength, len(response.Challenge))
	}
	response, err = c.sendUntilResponse(ctx, &uplink.Message{
		Type:      uplink.MessageTypeSubmitSolution,
		Challenge: clientChallenge,
		Solution:  uplink.SolveChallenge("client", password, response.Challenge),
	})
	switch {
	case err != nil:
		return err
	case response.Type == uplink.MessageTypeSubmitSolutionRejected:
		return os.ErrPermission
	case response.Type != uplink.MessageTypeSubmitSolutionAccepted:
		return fmt.Errorf("wrong response type from server: want %q, got %q", uplink.MessageTypeSubmitSolutionAccepted, response.Type)
	case !bytes.Equal(response.Solution, clientSolution):
		return fmt.Errorf("wrong solution from server to client challenge")
	}
	return nil
}

func Dial(ctx context.Context, addr, password string) (ipx.ReadWriteCloser, error) {
	udp, err := udpclient.Dial(addr)
	if err != nil {
		return nil, err
	}
	c := &client{
		inner:  udp,
		rxpipe: pipe.New(1),
	}
	if err := c.handshakeConnect(ctx, password); err != nil {
		udp.Close()
		return nil, err
	}
	go c.recvLoop(context.Background())
	return c, nil
}
