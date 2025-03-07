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
	"fmt"
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
	_ = (ipx.ReadWriteCloser)(&client{})
	_ = (server.Protocol)(&Protocol{})

	// Address is a special IPX address used to identify control packets;
	// control packets have this destination address.
	Address = ipx.Addr{'U', 'p', 'L', 'i', 'N', 'K'}
)

const (
	// MessageTypeGetChallengeRequest is the uplink message type initially
	// sent from client to server, requesting a challenge nonce. No other
	// field is set.
	// {"message-type": "get-challenge-request"}
	MessageTypeGetChallengeRequest = "get-challenge-request"

	// MessageTypeGetChallengeResponse is the uplink message type returned
	// by the server in response to MessageTypeGetChallengeRequest.
	// {"message-type": "get-challenge-response",
	//  "challenge": "[base64 challenge bytes]"}
	MessageTypeGetChallengeResponse = "get-challenge-response"

	// MessageTypeSubmitSolution is the uplink message type sent from the
	// client to server submitting its solution to the challenge from the
	// server. It also contains its own reverse-challenge to the server.
	// {"message-type": "submit-solution",
	//  "solution": "[base64 solution to server challenge]",
	//  "challenge": "[base64 challenge bytes]"}
	MessageTypeSubmitSolution = "submit-solution"

	// MessageTypeSubmitSolutionAccepted is the uplink message type sent
	// from the server to client confirming it accepts the client's
	// solution to the challenge. It also contains its own solution to the
	// client's challenge. At this point the server has confirmed
	// authentication of the client and will begin allowing traffic.
	// {"message-type": "submit-solution-accepted",
	//  "solution": "[base64 solution to client challenge]"}
	MessageTypeSubmitSolutionAccepted = "submit-solution-accepted"

	// MessageTypeSubmitSolutionRejected is the uplink message type sent
	// from the server to the client when the client's solution is not
	// accepted. Essentially this is wrong password, authentication
	// rejected.
	// {"message-type": "submit-solution-rejected"}
	MessageTypeSubmitSolutionRejected = "submit-solution-rejected"

	// MessageTypeKeepalive is the uplink message type sent by the server
	// when no traffic has been detected recently. It prevents any NAT
	// gateway in the middle from timing out the connection.
	MessageTypeKeepalive = "keepalive"

	// MessageTypeClose is the uplink message type from the client to
	// the server to close the connection and disconnect.
	// {"message-type": "close-connection"}
	MessageTypeClose = "close-connection"
)

const (
	MinChallengeLength = 64
)

type Message struct {
	Type      string `json:"message-type"`
	Challenge []byte `json:"challenge",omitempty`
	Solution  []byte `json:"solution",omitempty`
}

func (m *Message) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

func (m *Message) Unmarshal(data []byte) error {
	return json.Unmarshal(data, m)
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

	// If non-zero, always send at least one packet every few seconds to
	// keep the UDP connection open. Some NAT networks and firewalls can be
	// very aggressive about closing off the ability for clients to receive
	// packets on particular ports if nothing is received for a while.
	// This controls the time for keepalives.
	KeepaliveTime time.Duration
}

func (p *Protocol) log(format string, args ...interface{}) {
	if p.Logger != nil {
		p.Logger.Printf(format, args...)
	}
}

// IsRegistrationPacket returns true if this is an uplink packet of type
// MessageTypeGetChallengeRequest, which is the opening packet of a
// connection handshake.
func (p *Protocol) IsRegistrationPacket(packet *ipx.Packet) bool {
	if packet.Header.Dest.Addr != Address {
		return false
	}
	var msg Message
	if err := msg.Unmarshal(packet.Payload); err != nil {
		return false
	}
	return msg.Type == MessageTypeGetChallengeRequest
}

// StartClient is invoked as a new goroutine when a new client connects.
func (p *Protocol) StartClient(ctx context.Context, inner ipx.ReadWriteCloser, remoteAddr net.Addr) error {
	c := &client{
		p:             p,
		inner:         inner,
		authenticated: false,
		challenge:     make([]byte, MinChallengeLength),
		addr:          remoteAddr,
	}
	p.log("new uplink client from %s", remoteAddr)
	if _, err := rand.Read(c.challenge); err != nil {
		return err
	}
	node, err := p.Network.NewNode()
	if err != nil {
		return err
	}

	go c.sendKeepalives(ctx)
	defer func() {
		node.Close()
		statsString := stats.Summary(node)
		if statsString != "" {
			p.log("uplink client %s: final statistics: %s",
				remoteAddr.String(), statsString)
		}
	}()
	return ipx.DuplexCopyPackets(ctx, c, node)
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
	lastSendTime  time.Time
}

func (c *client) sendKeepalives(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(c.p.KeepaliveTime / 2):
		}
		c.mu.Lock()
		idleTime := time.Since(c.lastSendTime)
		isAuthenticated := c.authenticated
		c.mu.Unlock()
		if isAuthenticated && idleTime > c.p.KeepaliveTime {
			c.sendUplinkMessage(&Message{
				Type: MessageTypeKeepalive,
			})
		}
	}
}

func SolveChallenge(side, password string, challenge []byte) []byte {
	hashData := append([]byte(side), challenge...)
	hashData = append(hashData, []byte(password)...)
	hashData = append(hashData, challenge...)
	solution := sha256.Sum256(hashData)
	return solution[:]
}

func (c *client) sendUplinkMessage(msg *Message) error {
	jsonData, err := msg.Marshal()
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
		c.Close()
		return c.sendUplinkMessage(&Message{
			Type: MessageTypeSubmitSolutionRejected,
		})
	}
	c.mu.Lock()
	if !c.authenticated {
		c.p.log("uplink from %s authenticated successfully", c.addr)
		c.authenticated = true
		// Don't send a keepalive immediately.
		c.lastSendTime = time.Now()
	}
	c.mu.Unlock()
	return c.sendUplinkMessage(&Message{
		Type:     MessageTypeSubmitSolutionAccepted,
		Solution: SolveChallenge("server", c.p.Password, msg.Challenge),
	})
}

func (c *client) handleUplinkPacket(packet *ipx.Packet) error {
	var msg Message
	if err := msg.Unmarshal(packet.Payload); err != nil {
		return err
	}
	switch msg.Type {
	case MessageTypeGetChallengeRequest:
		return c.sendUplinkMessage(&Message{
			Type:      MessageTypeGetChallengeResponse,
			Challenge: c.challenge,
		})
	case MessageTypeSubmitSolution:
		return c.authenticate(&msg)
	case MessageTypeClose:
		c.p.log("uplink client %s closed connection", c.addr)
		c.Close()
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
	c.mu.Lock()
	c.lastSendTime = time.Now()
	c.mu.Unlock()
	return c.inner.WritePacket(packet)
}

func (c *client) Close() error {
	return c.inner.Close()
}
