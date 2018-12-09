// Package server implements the server side of the DOSBox IPX protocol.
package server

import (
	"math/rand"
	"net"
	"time"

	"github.com/fragglet/ipxbox/ipx"
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
}

// client represents a client that is connected to an IPX server.
type client struct {
	addr            *net.UDPAddr
	ipxAddr         ipx.Addr
	lastReceiveTime time.Time
	lastSendTime    time.Time
}

// Server is the top-level struct representing an IPX server that listens
// on a UDP port.
type Server struct {
	config           *Config
	socket           *net.UDPConn
	clients          map[string]*client
	clientsByIPX     map[ipx.Addr]*client
	timeoutCheckTime time.Time
}

var (
	DefaultConfig = &Config{
		ClientTimeout: 10 * time.Minute,
		KeepaliveTime: 5 * time.Second,
	}

	addrPingReply = [6]byte{0xff, 0xff, 0xff, 0xff, 0x00, 0x00}
)

// newAddress allocates a new random address that does not share an
// address with an existing client.
func (s *Server) newAddress() ipx.Addr {
	var result ipx.Addr

	// Repeatedly generate a new IPX address until we generate
	// one that is not already in use.
	for {
		for i := 0; i < len(result); i++ {
			result[i] = byte(rand.Intn(255))
		}

		// Never assign one of the special addresses.
		if result == ipx.AddrNull || result == ipx.AddrBroadcast || result == addrPingReply {
			continue
		}

		if _, ok := s.clientsByIPX[result]; !ok {
			break
		}
	}

	return result
}

// newClient processes a registration packet, adding a new client if necessary.
func (s *Server) newClient(header *ipx.Header, addr *net.UDPAddr) {
	addrStr := addr.String()
	c, ok := s.clients[addrStr]

	if !ok {
		//fmt.Printf("%s: %s: New client\n", now, addr)

		c = &client{
			addr:            addr,
			ipxAddr:         s.newAddress(),
			lastReceiveTime: time.Now(),
		}

		s.clients[addrStr] = c
		s.clientsByIPX[c.ipxAddr] = c
	}

	// Send a reply back to the client
	reply := &ipx.Header{
		Checksum:     0xffff,
		Length:       30,
		TransControl: 0,
		Dest: ipx.HeaderAddr{
			Network: [4]byte{0, 0, 0, 0},
			Addr:    c.ipxAddr,
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

// forwardPacket takes a packet received from a client and forwards it on
// to another client.
func (s *Server) forwardPacket(header *ipx.Header, packet []byte) {
	// We can only forward it on if the destination IPX address corresponds
	// to a client that we know about:
	if c, ok := s.clientsByIPX[header.Dest.Addr]; ok {
		c.lastSendTime = time.Now()
		s.socket.WriteToUDP(packet, c.addr)
	}
}

// forwardBroadcastPacket takes a broadcast packet received from a client and
// forwards it to all other clients.
func (s *Server) forwardBroadcastPacket(header *ipx.Header, packet []byte) {

	for _, c := range s.clients {
		if c.ipxAddr != header.Src.Addr {
			c.lastSendTime = time.Now()
			s.socket.WriteToUDP(packet, c.addr)
		}
	}
}

// processPacket decodes and processes a received UDP packet, sending responses
// and forwarding the packet on to other clients as appropriate.
func (s *Server) processPacket(packet []byte, addr *net.UDPAddr) {
	var header ipx.Header
	if err := header.UnmarshalBinary(packet); err != nil {
		return
	}

	if header.IsRegistrationPacket() {
		s.newClient(&header, addr)
		return
	}

	srcClient, ok := s.clients[addr.String()]
	if !ok {
		return
	}

	// Clients can only send from their own address.
	if header.Src.Addr != srcClient.ipxAddr {
		return
	}

	srcClient.lastReceiveTime = time.Now()

	if header.IsBroadcast() {
		s.forwardBroadcastPacket(&header, packet)
	} else {
		s.forwardPacket(&header, packet)
	}
}

// New creates a new Server.
func New(c *Config) *Server {
	return &Server{
		config: c,
	}
}

// Listen initializes a Server struct, binding to the given address
// so that we can receive packets.
func (s *Server) Listen(addr string) error {
	udp4Addr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return err
	}

	socket, err := net.ListenUDP("udp", udp4Addr)
	if err != nil {
		return err
	}

	s.socket = socket
	s.clients = map[string]*client{}
	s.clientsByIPX = map[ipx.Addr]*client{}
	s.timeoutCheckTime = time.Now().Add(10e9)

	return nil
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

	for _, c := range s.clients {
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
			delete(s.clients, c.addr.String())
			delete(s.clientsByIPX, c.ipxAddr)
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

// Poll listens for new packets, blocking until one is received, or until
// a timeout is reached.
func (s *Server) Poll() error {
	var buf [1500]byte

	s.socket.SetReadDeadline(s.timeoutCheckTime)

	packetLen, addr, err := s.socket.ReadFromUDP(buf[0:])

	if err == nil {
		s.processPacket(buf[0:packetLen], addr)
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

