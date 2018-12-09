//
// A standalone DOSbox IPX-over-UDP server.
//

package main

import (
	"fmt"
	"math/rand"
	"net"
	"os"
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

// Client represents a client that is connected to an IPX server.
type Client struct {
	addr            *net.UDPAddr
	ipxAddr         ipx.Addr
	lastReceiveTime time.Time
	lastSendTime    time.Time
}

// IPXServer is the top-level struct representing an IPX server that listens
// on a UDP port.
type IPXServer struct {
	config           *Config
	socket           *net.UDPConn
	clients          map[string]*Client
	clientsByIPX     map[ipx.Addr]*Client
	timeoutCheckTime time.Time
}

var (
	DefaultConfig = &Config{
		ClientTimeout: 10 * time.Minute,
		KeepaliveTime: 5 * time.Second,
	}

	ADDR_PINGREPLY = [6]byte{0xff, 0xff, 0xff, 0xff, 0x00, 0x00}
)

// NewAddress allocates a new random address that does not share an
// address with an existing client.
func (server *IPXServer) NewAddress() ipx.Addr {
	var result ipx.Addr

	// Repeatedly generate a new IPX address until we generate
	// one that is not already in use.
	for {
		for i := 0; i < len(result); i++ {
			result[i] = byte(rand.Intn(255))
		}

		// Never assign one of the special addresses.
		if result == ipx.AddrNull || result == ipx.AddrBroadcast ||
			result == ADDR_PINGREPLY {
			continue
		}

		if _, ok := server.clientsByIPX[result]; !ok {
			break
		}
	}

	return result
}

// NewClient processes a registration packet, adding a new client if necessary.
func (server *IPXServer) NewClient(header *ipx.Header, addr *net.UDPAddr) {
	addrStr := addr.String()
	client, ok := server.clients[addrStr]

	if !ok {
		//fmt.Printf("%s: %s: New client\n", now, addr)

		client = &Client{
			addr:            addr,
			ipxAddr:         server.NewAddress(),
			lastReceiveTime: time.Now(),
		}

		server.clients[addrStr] = client
		server.clientsByIPX[client.ipxAddr] = client
	}

	// Send a reply back to the client
	reply := &ipx.Header{
		Checksum:     0xffff,
		Length:       30,
		TransControl: 0,
		Dest: ipx.HeaderAddr{
			Network: [4]byte{0, 0, 0, 0},
			Addr:    client.ipxAddr,
			Socket:  2,
		},
		Src: ipx.HeaderAddr{
			Network: [4]byte{0, 0, 0, 1},
			Addr:    ipx.AddrBroadcast,
			Socket:  2,
		},
	}

	client.lastSendTime = time.Now()
	encodedReply, err := reply.MarshalBinary()
	if err == nil {
		server.socket.WriteToUDP(encodedReply, client.addr)
	}
}

// ForwardPacket takes a packet received from a client and forwards it on
// to another client.
func (server *IPXServer) ForwardPacket(header *ipx.Header, packet []byte) {
	// We can only forward it on if the destination IPX address corresponds
	// to a client that we know about:
	if client, ok := server.clientsByIPX[header.Dest.Addr]; ok {
		client.lastSendTime = time.Now()
		server.socket.WriteToUDP(packet, client.addr)
	}
}

// ForwardBroadcastPacket takes a broadcast packet received from a client and
// forwards it to all other clients.
func (server *IPXServer) ForwardBroadcastPacket(header *ipx.Header,
	packet []byte) {

	for _, client := range server.clients {
		if client.ipxAddr != header.Src.Addr {
			client.lastSendTime = time.Now()
			server.socket.WriteToUDP(packet, client.addr)
		}
	}
}

// ProcessPacket decodes and processes a received UDP packet, sending responses
// and forwarding the packet on to other clients as appropriate.
func (server *IPXServer) ProcessPacket(packet []byte, addr *net.UDPAddr) {
	var header ipx.Header
	if err := header.UnmarshalBinary(packet); err != nil {
		return
	}

	if header.IsRegistrationPacket() {
		server.NewClient(&header, addr)
		return
	}

	srcClient, ok := server.clients[addr.String()]
	if !ok {
		return
	}

	// Clients can only send from their own address.
	if header.Src.Addr != srcClient.ipxAddr {
		return
	}

	srcClient.lastReceiveTime = time.Now()

	if header.IsBroadcast() {
		server.ForwardBroadcastPacket(&header, packet)
	} else {
		server.ForwardPacket(&header, packet)
	}
}

// Listen initializes an IPXServer struct, binding to the given address
// so that we can receive packets.
func (server *IPXServer) Listen(addr string) bool {
	udp4Addr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve address: %v\n", err)
		return false
	}

	socket, err := net.ListenUDP("udp", udp4Addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open socket: %v\n", err)
		return false
	}

	server.socket = socket
	server.clients = map[string]*Client{}
	server.clientsByIPX = map[ipx.Addr]*Client{}
	server.timeoutCheckTime = time.Now().Add(10e9)

	return true
}

// SendPing transmits a ping packet to the given client. The DOSbox IPX client
// code recognizes broadcast packets sent to socket=2 and will send a reply to
// the source address that we provide.
func (server *IPXServer) SendPing(client *Client) {
	header := &ipx.Header{
		Dest: ipx.HeaderAddr{
			Addr:   ipx.AddrBroadcast,
			Socket: 2,
		},
		// We "send" the pings from an imaginary "ping reply" address
		// because if we used ipx.AddrNull the reply would be
		// indistinguishable from a registration packet.
		Src: ipx.HeaderAddr{
			Addr:   ADDR_PINGREPLY,
			Socket: 0,
		},
	}

	client.lastSendTime = time.Now()
	encodedHeader, err := header.MarshalBinary()
	if err == nil {
		server.socket.WriteToUDP(encodedHeader, client.addr)
	}
}

// CheckClientTimeouts checks all clients that are connected to the server and
// handles idle clients to which we have no sent data or from which we have not
// received data recently. This function should be called regularly; it returns
// the time that it should next be invoked.
func (server *IPXServer) CheckClientTimeouts() time.Time {
	now := time.Now()

	// At absolute max we should check again in 10 seconds, as a new client
	// might connect in the mean time.
	nextCheckTime := now.Add(10 * time.Second)

	for _, client := range server.clients {
		// Nothing sent in a while? Send a keepalive.
		// This is important because some types of game use a
		// client/server type arrangement where the server does not
		// broadcast anything but listens for broadcasts from clients.
		// An example is Warcraft 2. If there is no activity between
		// the client and server in a long time, some NAT gateways or
		// firewalls can drop the association.
		keepaliveTime := client.lastSendTime.Add(server.config.KeepaliveTime)
		if now.After(keepaliveTime) {
			// We send a keepalive in the form of a ping packet
			// that the client should respond to, thus keeping us
			// from timing out the client from our own table if it
			// really is still there.
			server.SendPing(client)
			keepaliveTime = client.lastSendTime.Add(server.config.KeepaliveTime)
		}

		// Nothing received in a long time? Time out the connection.
		timeoutTime := client.lastReceiveTime.Add(server.config.ClientTimeout)
		if now.After(timeoutTime) {
			//fmt.Printf("%s: %s: Client timed out\n",
			//	now, client.addr)
			delete(server.clients, client.addr.String())
			delete(server.clientsByIPX, client.ipxAddr)
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
func (server *IPXServer) Poll() bool {
	var buf [1500]byte

	server.socket.SetReadDeadline(server.timeoutCheckTime)

	packetLen, addr, err := server.socket.ReadFromUDP(buf[0:])

	if err == nil {
		server.ProcessPacket(buf[0:packetLen], addr)
	} else if nerr, ok := err.(net.Error); ok && !nerr.Timeout() {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return false
	}

	// We must regularly call CheckClientTimeouts(); when we do, update
	// server.timeoutCheckTime with the next time it should be invoked.
	if time.Now().After(server.timeoutCheckTime) {
		server.timeoutCheckTime = server.CheckClientTimeouts()
	}

	return true
}

func main() {
	server := &IPXServer{
		config: DefaultConfig,
	}
	if !server.Listen(":10000") {
		os.Exit(1)
	}

	for {
		if !server.Poll() {
			os.Exit(1)
		}
	}
}
