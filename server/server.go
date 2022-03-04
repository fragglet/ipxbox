// Package server implements the server side of the DOSBox IPX protocol.
package server

import (
	"context"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/network/pipe"
	"github.com/fragglet/ipxbox/network/stats"
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
	rxpipe          ipx.ReadWriteCloser
	addr            *net.UDPAddr
	node            network.Node
	lastReceiveTime time.Time
	lastSendTime    time.Time
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
	return nil // TODO
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

	// Server-initiated pings come from this address.
	addrPingReply = [6]byte{0x02, 0xff, 0xff, 0xff, 0x00, 0x00}

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

// runClient continually copies packets from the client's node and sends them
// to the connected UDP client. The function will only return when the client's
// network node is Close()d.
func (s *Server) runClient(ctx context.Context, c *client) {
	if err := ipx.DuplexCopyPackets(ctx, c, c.node); err != nil {
		s.log("client %s: error while copying packets: %v", c.addr.String(), err)
	}
}

func (s *Server) log(format string, args ...interface{}) {
	if s.config.Logger != nil {
		s.config.Logger.Printf(format, args...)
	}
}

// newClient processes a registration packet, adding a new client if necessary.
func (s *Server) newClient(ctx context.Context, header *ipx.Header, addr *net.UDPAddr) {
	addrStr := addr.String()
	c, ok := s.clients[addrStr]

	if !ok {
		now := time.Now()
		c = &client{
			s:               s,
			rxpipe:          pipe.New(1),
			addr:            addr,
			lastReceiveTime: now,
			node:            s.net.NewNode(),
		}

		s.clients[addrStr] = c
		s.log("new connection from %s, assigned IPX address %s",
			addrStr, network.NodeAddress(c.node))
		// TODO: Use cancellable context for client disconnect?
		go s.runClient(ctx, c)
	}

	// Send a reply back to the client
	reply := &ipx.Header{
		Checksum:     0xffff,
		Length:       30,
		TransControl: 0,
		Dest: ipx.HeaderAddr{
			Network: [4]byte{0, 0, 0, 0},
			Addr:    network.NodeAddress(c.node),
			Socket:  2,
		},
		Src: ipx.HeaderAddr{
			Network: [4]byte{0, 0, 0, 1},
			Addr:    ipx.AddrBroadcast,
			Socket:  2,
		},
	}

	c.lastSendTime = time.Now()
	encodedReply, err := reply.MarshalBinary()
	if err == nil {
		s.socket.WriteToUDP(encodedReply, c.addr)
	}
}

// processPacket decodes and processes a received UDP packet, sending responses
// and forwarding the packet on to other clients as appropriate.
func (s *Server) processPacket(ctx context.Context, packetBytes []byte, addr *net.UDPAddr) {
	packet := &ipx.Packet{}
	if err := packet.UnmarshalBinary(packetBytes); err != nil {
		return
	}

	if packet.Header.IsRegistrationPacket() {
		s.newClient(ctx, &packet.Header, addr)
		return
	}

	// Find which client sent it; it must be a registered client sending
	// from their own IPX address.
	srcClient, ok := s.clients[addr.String()]
	if !ok {
		return
	}

	// Deliver packet to the network.
	srcClient.lastReceiveTime = time.Now()
	srcClient.rxpipe.WritePacket(packet)
}

// sendPing transmits a ping packet to the given client. The DOSbox IPX client
// code recognizes broadcast packets sent to socket=2 and will send a reply to
// the source address that we provide.
func (s *Server) sendPing(c *client) {
	header := &ipx.Header{
		Dest: ipx.HeaderAddr{
			Addr:   ipx.AddrBroadcast,
			Socket: 2,
		},
		// We "send" the pings from an imaginary "ping reply" address
		// because if we used ipx.AddrNull the reply would be
		// indistinguishable from a registration packet.
		Src: ipx.HeaderAddr{
			Addr:   addrPingReply,
			Socket: 0,
		},
	}

	c.lastSendTime = time.Now()
	encodedHeader, err := header.MarshalBinary()
	if err == nil {
		s.socket.WriteToUDP(encodedHeader, c.addr)
	}
}

// checkClientTimeouts checks all clients that are connected to the server and
// handles idle clients to which we have no sent data or from which we have not
// received data recently. This function should be called regularly; it returns
// the time that it should next be invoked.
func (s *Server) checkClientTimeouts() time.Time {
	now := time.Now()

	// At absolute max we should check again in 10 seconds, as a new client
	// might connect in the mean time.
	nextCheckTime := now.Add(10 * time.Second)

	for addrStr, c := range s.clients {
		// Nothing sent in a while? Send a keepalive.
		// This is important because some types of game use a
		// client/server type arrangement where the server does not
		// broadcast anything but listens for broadcasts from clients.
		// An example is Warcraft 2. If there is no activity between
		// the client and server in a long time, some NAT gateways or
		// firewalls can drop the association.
		keepaliveTime := c.lastSendTime.Add(s.config.KeepaliveTime)
		if now.After(keepaliveTime) {
			// We send a keepalive in the form of a ping packet
			// that the client should respond to, thus keeping us
			// from timing out the client from our own table if it
			// really is still there.
			s.sendPing(c)
			keepaliveTime = c.lastSendTime.Add(s.config.KeepaliveTime)
		}

		// Nothing received in a long time? Time out the connection.
		timeoutTime := c.lastReceiveTime.Add(s.config.ClientTimeout)
		if now.After(timeoutTime) {
			s.log(("client %s (IPX address %s) timed out: " +
				"nothing received since %s. %s"),
				addrStr, network.NodeAddress(c.node),
				c.lastReceiveTime, stats.Summary(c.node))
			delete(s.clients, c.addr.String())
			c.node.Close()
			c.rxpipe.Close()
		}

		if keepaliveTime.Before(nextCheckTime) {
			nextCheckTime = keepaliveTime
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
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, client := range s.clients {
		client.node.Close()
		client.rxpipe.Close()
	}
	return s.socket.Close()
}
