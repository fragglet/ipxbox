// Package server implements a server that sends and receives IPX frames
// inside UDP packets.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network/pipe"
)

var (
	_ = (ipx.ReadWriteCloser)(&client{})
	_ = (io.Closer)(&Server{})
)

// Config contains configuration parameters for an IPX server.
type Config struct {
	// Protocol contains the implementation of the inner protocol
	// logic.
	Protocols []Protocol

	// Clients time out if nothing is received for this amount of time.
	ClientTimeout time.Duration

	// If not nil, log entries are written as clients connect and
	// disconnect.
	Logger *slog.Logger
}

// Protocol implements the inner protocol logic of the server.
type Protocol interface {
	// StartClient is invoked each time the server receives packets from
	// a new address. The method call happens in its own goroutine and
	// is passed an ipx.ReadWriteCloser that can be used to send and
	// receive packets to the client. Returning from the method call
	// closes the connection.
	StartClient(context.Context, ipx.ReadWriteCloser, net.Addr) error

	// IsRegistrationPacket is invoked when a new client is created, to
	// determine if the client is attempting to connect with this protocol.
	// The function returns true if it is a valid registration packet.
	IsRegistrationPacket(*ipx.Packet) bool
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
	mu               sync.Mutex
	config           *Config
	socket           *net.UDPConn
	clients          map[string]*client
	timeoutCheckTime time.Time
}

func addrString(addr *net.UDPAddr) string {
	// TODO: If addr.IP is non-zero, just return addr.String()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "[unknown]"
	}
	result := []string{}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		// We only support IPv4; it's all dosbox supports.
		ip4 := ipnet.IP.To4()
		if ip4 == nil {
			continue
		}
		result = append(result, fmt.Sprintf("%s:%d", ip4.String(), addr.Port))
	}
	return strings.Join(result, ", or ")
}

// New creates a new Server, listening on the given address.
func New(addr string, c *Config) (*Server, error) {
	udp4Addr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}
	socket, err := net.ListenUDP("udp", udp4Addr)
	if err != nil {
		return nil, err
	}
	c.Logger.Info(fmt.Sprintf("Server started, connect on %s", addrString(udp4Addr)))
	return &Server{
		config:           c,
		socket:           socket,
		clients:          map[string]*client{},
		timeoutCheckTime: time.Now().Add(10 * time.Second),
	}, nil
}

// findProtocol checks the protocols supported by the server and returns
// a Protocol that matches the given packet. If no valid protocols are
// found then nil, false is returned.
func (s *Server) findProtocol(packet *ipx.Packet) (Protocol, bool) {
	for _, proto := range s.config.Protocols {
		if proto.IsRegistrationPacket(packet) {
			return proto, true
		}
	}
	return nil, false
}

// AddrAttr returns a `slog.Attr` referencing the given `net.Addr`.
func AddrAttr(addr net.Addr) slog.Attr {
	return slog.String("address", addr.String())
}

// newClient is invoked when a new client should be started. When called, a
// packet has been received from the given address but no client matches the
// address.
func (s *Server) newClient(ctx context.Context, protocol Protocol, addr *net.UDPAddr) *client {
	addrStr := addr.String()
	now := time.Now()
	c := &client{
		s:               s,
		rxpipe:          pipe.New(),
		addr:            addr,
		lastReceiveTime: now,
	}
	s.clients[addrStr] = c

	go func() {
		subctx, cancel := context.WithCancel(ctx)

		err := protocol.StartClient(subctx, c, addr)

		if errors.Is(err, io.ErrClosedPipe) {
			err = nil
		}
		if err != nil {
			s.config.Logger.Error("client terminated abnormally",
				AddrAttr(addr),
				slog.String("error", err.Error()))
		}
		cancel()
		c.Close()
	}()
	return c
}

// processPacket decodes a received UDP packet, delivering it to the appropriate
// client based on address. A new client is started if none matches the address.
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
		// Is this a supported protocol?
		protocol, ok := s.findProtocol(packet)
		if !ok {
			s.mu.Unlock()
			return
		}

		srcClient = s.newClient(ctx, protocol, addr)
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
			s.config.Logger.Error("client timed out",
				AddrAttr(c.addr),
				slog.Time("last_receive_time", c.lastReceiveTime))
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
