// Package uplink implements the server side of the ipxbox uplink protocol.
// This is largely the same IPX-in-UDP protocol used by DOSbox, but there
// is a challenge-response authentication system to provide a bit more
// security since uplinked packets can be any MAC address.
package uplink

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/server"
)

var (
	_ = (ipx.ReadWriteCloser)(&client{})
	_ = (server.Protocol)(&Protocol{})

	// Address is a special IPX address used to identify control packets;
	// control packets have this destination address.
	Address = ipx.Addr{'U', 'p', 'L', 'i', 'N', 'K'}

	ErrAuthenticationRejected = errors.New("authentication rejected")
)

const (
	// messageTypeGetChallengeRequest is the uplink message type initially
	// sent from client to server, requesting a challenge nonce. No other
	// field is set.
	// {"message-type": "get-challenge-request"}
	messageTypeGetChallengeRequest = "get-challenge-request"

	// messageTypeGetChallengeResponse is the uplink message type returned
	// by the server in response to messageTypeGetChallengeRequest.
	// {"message-type": "get-challenge-response",
	//  "challenge": "[base64 challenge bytes]"}
	messageTypeGetChallengeResponse = "get-challenge-response"

	// messageTypeSubmitSolution is the uplink message type sent from the
	// client to server submitting its solution to the challenge from the
	// server. It also contains its own reverse-challenge to the server.
	// {"message-type": "submit-solution",
	//  "solution": "[base64 solution to server challenge]",
	//  "challenge": "[base64 challenge bytes]"}
	messageTypeSubmitSolution = "submit-solution"

	// messageTypeSubmitSolutionAccepted is the uplink message type sent
	// from the server to client confirming it accepts the client's
	// solution to the challenge. It also contains its own solution to the
	// client's challenge. At this point the server has confirmed
	// authentication of the client and will begin allowing traffic.
	// {"message-type": "submit-solution-accepted",
	//  "solution": "[base64 solution to client challenge]"}
	messageTypeSubmitSolutionAccepted = "submit-solution-accepted"
)

const (
	MinChallengeLength = 64
)

type Message struct {
	Type      string `json:"message-type"`
	Challenge []byte `json:"challenge",omitempty`
	Solution  []byte `json:"solution",omitempty`
}

// Protocol is an implementation of server.Protocol that provides uplink
// capability.
type Protocol struct {
	// A new Node is created in this network each time a client connects.
	// This should not be an Addressable network since for uplink we want
	// to allow traffic to and from any arbitrary address.
	Network network.Network

	// If not nil, log entries are written as clients connect and
	// disconnect.
	Logger *log.Logger

	// Clients *must* supply a password. Uplink is always authenticated.
	Password string
}

func (p *Protocol) log(format string, args ...interface{}) {
	if p.Logger != nil {
		p.Logger.Printf(format, args...)
	}
}

// StartClient is invoked as a new goroutine when a new client connects.
func (p *Protocol) StartClient(ctx context.Context, inner ipx.ReadWriteCloser, remoteAddr net.Addr) error {
	c := &client{
		p:         p,
		inner:     inner,
		challenge: make([]byte, MinChallengeLength),
		addr:      remoteAddr,
	}
	p.log("new uplink client from %s", remoteAddr)
	if _, err := rand.Read(c.challenge); err != nil {
		return err
	}
	// TODO
	return nil
}

// client implements the uplink protocol as a wrapper around an inner
// ReadWriteCloser that is used to send and receive packets.
type client struct {
	p             *Protocol
	inner         ipx.ReadWriteCloser
	authenticated bool
	challenge     []byte
	mu            sync.Mutex
	addr          net.Addr
}

func SolveChallenge(side, password string, challenge []byte) []byte {
	hashData := append([]byte(side), challenge...)
	hashData = append(hashData, []byte(password)...)
	hashData = append(hashData, challenge...)
	solution := sha256.Sum256(hashData)
	return solution[:]
}

func (c *client) sendUplinkMessage(msg *Message) error {
	jsonData, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.inner.WritePacket(&ipx.Packet{
		Header: ipx.Header{
			Dest: ipx.HeaderAddr{
				Addr: Address,
			},
		},
		Payload: jsonData,
	})
	return nil
}

func (c *client) authenticate(msg *Message) error {
	if len(msg.Challenge) < MinChallengeLength {
		return fmt.Errorf("client challenge too short: want minimum %d bytes, got %d", MinChallengeLength, len(msg.Challenge))
	}
	solution := SolveChallenge("client", c.p.Password, c.challenge)
	if !bytes.Equal(msg.Solution, solution) {
		c.p.log("uplink client %s authentication rejected", c.addr)
		// TODO: send fail response
		return ErrAuthenticationRejected
	}
	c.mu.Lock()
	if !c.authenticated {
		c.p.log("uplink from %s authenticated successfully", c.addr)
		c.authenticated = true
	}
	c.mu.Unlock()
	return c.sendUplinkMessage(&Message{
		Type:     messageTypeSubmitSolutionAccepted,
		Solution: SolveChallenge("server", c.p.Password, msg.Challenge),
	})
}

func (c *client) handleUplinkPacket(packet *ipx.Packet) error {
	var msg Message
	if err := json.Unmarshal(packet.Payload, &msg); err != nil {
		return err
	}
	switch msg.Type {
	case messageTypeGetChallengeRequest:
		return c.sendUplinkMessage(&Message{
			Type:      messageTypeGetChallengeResponse,
			Challenge: c.challenge,
		})
	case messageTypeSubmitSolution:
		return c.authenticate(&msg)
	}
	return nil
}

func (c *client) isAuthenticated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.authenticated
}

func (c *client) ReadPacket(ctx context.Context) (*ipx.Packet, error) {
	for {
		packet, err := c.inner.ReadPacket(ctx)
		if err != nil {
			return nil, err
		}
		if packet.Header.Dest.Addr == Address {
			c.handleUplinkPacket(packet)
		}

		// Packets get silently discarded until authenticated.
		if !c.isAuthenticated() {
			continue
		}
		return packet, nil
	}
}

func (c *client) WritePacket(packet *ipx.Packet) error {
	// Packets get silently discarded until authenticated.
	if !c.isAuthenticated() {
		return nil
	}
	return c.inner.WritePacket(packet)
}

func (c *client) Close() error {
	return c.inner.Close()
}
