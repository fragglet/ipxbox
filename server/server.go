// Package server implements the server side of the DOSBox IPX protocol.
package server

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/network/pipe"
	"github.com/fragglet/ipxbox/network/stats"
	dosbox "github.com/fragglet/ipxbox/server/dosbox"
)

var (
	_ = (ipx.ReadWriteCloser)(&client{})
)

// Config contains configuration parameters for an IPX server.
type Config struct {
	// Clients time out if nothing is received for this amount of time.
	ClientTimeout time.Duration

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

// client represents a client that is connected to an IPX server.
type client struct {
	s               *Server
	closed          bool
	rxpipe          ipx.ReadWriteCloser
	addr            *net.UDPAddr
	lastReceiveTime time.Time
}

func (c *client) ReadPacket(ctx context.Context) (*ipx.Packet, error) {
	return c.rxpipe.ReadPacket(ctx)
}

func (c *client) WritePacket(packet *ipx.Packet) error {
	packetBytes, err := packet.MarshalBinary()
	if err != nil {
		return err
	}
	_, err = c.s.socket.WriteToUDP(packetBytes, c.addr)
	return err
}

func (c *client) Close() error {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	if !c.closed {
		delete(c.s.clients, c.addr.String())
		c.closed = true
	}
	return c.rxpipe.Close()
}

// Server is the top-level struct representing an IPX server that listens
// on a UDP port.
type Server struct {
	net              network.Network
	mu               sync.Mutex
	config           *Config
	socket           *net.UDPConn
	clients          map[string]*client
	timeoutCheckTime time.Time
}

var (
	DefaultConfig = &Config{
		ClientTimeout: 10 * time.Minute,
		KeepaliveTime: 5 * time.Second,
	}

	_ = (io.Closer)(&Server{})
)

// New creates a new Server, listening on the given address.
func New(addr string, n network.Network, c *Config) (*Server, error) {
	udp4Addr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}
	socket, err := net.ListenUDP("udp", udp4Addr)
	if err != nil {
		return nil, err
	}
	s := &Server{
		net:              n,
		config:           c,
		socket:           socket,
		clients:          map[string]*client{},
		timeoutCheckTime: time.Now().Add(10e9),
	}
	return s, nil
}

// startClient is invoked as a new goroutine when a new client connects.
// TODO: Move to dosbox protocol package
func startClient(ctx context.Context, c *client) error {
	packet, err := c.ReadPacket(ctx)
	if err != nil {
		return err
	}
	if !packet.Header.IsRegistrationPacket() {
		return nil
	}
	node := c.s.net.NewNode()
	nodeAddr := network.NodeAddress(node)
	defer func() {
		node.Close()
		statsString := stats.Summary(node)
		if statsString != "" {
			c.s.log("%s (IPX address %s): final statistics: %s",
				c.addr.String(), nodeAddr.String(), statsString)
		}
	}()

	c.s.log("%s: new connection, assigned IPX address %s",
		c.addr.String(), network.NodeAddress(node))
	p := dosbox.MakeClient(c, &nodeAddr)
	p.SendRegistrationReply()

	go p.SendKeepalives(ctx, c.s.config.KeepaliveTime)

	return ipx.DuplexCopyPackets(ctx, p, node)
}

func (s *Server) log(format string, args ...interface{}) {
	if s.config.Logger != nil {
		s.config.Logger.Printf(format, args...)
	}
}

// newClient processes a registration packet, adding a new client if necessary.
func (s *Server) newClient(ctx context.Context, packet *ipx.Packet, addr *net.UDPAddr) *client {
	addrStr := addr.String()
	now := time.Now()
	c := &client{
		s:               s,
		rxpipe:          pipe.New(1),
		addr:            addr,
		lastReceiveTime: now,
	}
	s.clients[addrStr] = c

	go func() {
		subctx, cancel := context.WithCancel(ctx)

		err := startClient(subctx, c)

		if errors.Is(err, io.ErrClosedPipe) {
			err = nil
		}
		if err != nil {
			s.log("client %s terminated abnormally: %v", addrStr, err)
		}
		cancel()
		c.Close()
	}()
	return c
}

// processPacket decodes and processes a received UDP packet, sending responses
// and forwarding the packet on to other clients as appropriate.
func (s *Server) processPacket(ctx context.Context, packetBytes []byte, addr *net.UDPAddr) {
	packet := &ipx.Packet{}
	if err := packet.UnmarshalBinary(packetBytes); err != nil {
		return
	}

	// Find which client sent it, and forward to receive queue.
	// If we don't find a client matching this address, start a new one.
	s.mu.Lock()
	srcClient, ok := s.clients[addr.String()]
	if !ok {
		srcClient = s.newClient(ctx, packet, addr)
	}
	s.mu.Unlock()

	srcClient.lastReceiveTime = time.Now()
	srcClient.rxpipe.WritePacket(packet)
}

func (s *Server) allClients() []*client {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := []*client{}
	for _, c := range s.clients {
		result = append(result, c)
	}
	return result
}

// checkClientTimeouts checks all clients connected to the server and
// disconnects idle clients we have not received data from recently. This
// function should be called regularly; it returns the time that it should next
// be invoked.
func (s *Server) checkClientTimeouts() time.Time {
	now := time.Now()

	// At absolute max we should check again in 10 seconds, as a new client
	// might connect in the mean time.
	nextCheckTime := now.Add(10 * time.Second)

	for _, c := range s.allClients() {
		// Nothing received in a long time? Time out the connection.
		timeoutTime := c.lastReceiveTime.Add(s.config.ClientTimeout)
		if now.After(timeoutTime) {
			s.log(("client %s timed out: nothing received " +
				"since %s."),
				c.addr.String(), c.lastReceiveTime)
			c.Close()
		}

		if timeoutTime.Before(nextCheckTime) {
			nextCheckTime = timeoutTime
		}
	}

	return nextCheckTime
}

// poll listens for new packets, blocking until one is received, or until
// a timeout is reached.
func (s *Server) poll(ctx context.Context) error {
	var buf [1500]byte

	s.socket.SetReadDeadline(s.timeoutCheckTime)
	packetLen, addr, err := s.socket.ReadFromUDP(buf[:])

	if err == nil {
		s.processPacket(ctx, buf[0:packetLen], addr)
	} else if nerr, ok := err.(net.Error); ok && !nerr.Timeout() {
		return err
	}

	// We must regularly call checkClientTimeouts(); when we do, update
	// server.timeoutCheckTime with the next time it should be invoked.
	if time.Now().After(s.timeoutCheckTime) {
		s.timeoutCheckTime = s.checkClientTimeouts()
	}

	return nil
}

// Run runs the server, blocking until the socket is closed or an error occurs.
func (s *Server) Run(ctx context.Context) {
	for {
		if err := s.poll(ctx); err != nil {
			return
		}
	}
}

// Close closes the socket associated with the server to shut it down.
func (s *Server) Close() error {
	for _, client := range s.allClients() {
		client.Close()
	}
	return s.socket.Close()
}
