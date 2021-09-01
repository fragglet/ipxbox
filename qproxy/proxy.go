// Package qproxy implements a proxy client that makes Quake UDP servers
// available on an IPX network.
package qproxy

import (
	"context"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
)

const (
	garbageCollectPeriod = 10 * time.Second
	quakeIPXSocket       = 26000
	connectedIPXSocket   = 26001
	quakeHeaderBytes     = 4
	acceptHeaderMinLen   = 9

	// Packet response from server when accepting connection
	ccRepAccept = 0x81
)

type Config struct {
	// Address of Quake server.
	Address string

	// IdleTimeout is the amount of time after which a connection is deleted.
	IdleTimeout time.Duration
}

type connection struct {
	p             *Proxy
	rs reliableSharder
	ipxAddr       *ipx.HeaderAddr
	conn          *net.UDPConn
	lastRXTime    time.Time
	connectedPort int
	ipxSocket     uint16
	closed        bool
}

// handleAccept checks if a packet received from the main server port is a
// CCREP_ACCEPT packet, and if so, reads the connected port number from the
// packet, then replaces it with connectedIPXSocket.
func (c *connection) handleAccept(packet []byte, serverAddr *net.UDPAddr) {
	if len(packet) < acceptHeaderMinLen {
		return
	}
	if packet[4] != ccRepAccept {
		return
	}
	// We have a legitimate looking CCREP_ACCEPT packet.
	// The server has indicated the port number assigned for this
	// connection as part of the packet.
	c.connectedPort = (int(packet[6]) << 8) | int(packet[5])
	// Some Quake source ports do not allocate a new port per connection.
	// In this case we cannot distinguish between packets destined for
	// quakeIPXSocket vs connectedIPXSocket. Therefore in this case we
	// forward all traffic from the same IPX port.
	if c.connectedPort == serverAddr.Port {
		c.ipxSocket = quakeIPXSocket
	}
	// Before forwarding onto the IPX network, we must replace the UDP
	// socket number with the connected IPX port number.
	packet[5] = byte(c.ipxSocket & 0xff)
	packet[6] = byte((c.ipxSocket >> 8) & 0xff)
	// The server will try to send us packets from the new port, but we
	// may be behind a firewall connecting outwards. So send a packet to
	// this new port so that packets will get through.
	if c.connectedPort != serverAddr.Port {
		destAddress := &net.UDPAddr{
			IP:   serverAddr.IP,
			Port: c.connectedPort,
		}
		if _, err := c.conn.WriteToUDP([]byte{}, destAddress); err != nil {
			log.Printf("error sending firewall traversal packet: %v", err)
		}
	}
}

func (c *connection) sendToDownstreamSocket(payload []byte, socket uint16) error {
	zeroBytes := [quakeHeaderBytes]byte{}
	pktBytes := append([]byte{}, zeroBytes[:]...)
	pktBytes = append(pktBytes, payload...)
	return c.p.node.WritePacket(&ipx.Packet{
		Header: ipx.Header{
			Length: uint16(ipx.HeaderLength + len(pktBytes)),
			Dest:   *c.ipxAddr,
			Src: ipx.HeaderAddr{
				Addr:   network.NodeAddress(c.p.node),
				Socket: socket,
			},
		},
		Payload: pktBytes,
	})
}

// sendToDownstream forwards the given packet to the client, sending to the
// client's data socket.
func (c *connection) sendToDownstream(payload []byte) error {
	return c.sendToDownstreamSocket(payload, c.ipxSocket)
}

// sendToUpstream forwards the given packet to the UDP port of the server.
func (c *connection) sendToUpstream(payload []byte) error {
	if c.connectedPort < 0 {
		return nil
	}
	_, err := c.conn.WriteToUDP(payload, &net.UDPAddr{
		IP:   c.p.address.IP,
		Port: c.connectedPort,
	})
	return err
}

func (c *connection) receivePackets() {
	var buf [9000]byte
	for {
		n, addr, err := c.conn.ReadFromUDP(buf[:])
		switch {
		case c.closed:
			return
		case err != nil:
			log.Printf("error receiving UDP packets for connection to %v: %v", c.conn.RemoteAddr(), err)
			return
		}
		// Sanity check: packet must come from server's IP address.
		if !addr.IP.Equal(c.p.address.IP) {
			continue
		}
		// Packet must come from either the server's main port or from
		// the port assigned to this connection. Map this into the IPX
		// socket number for the source address.
		var socket uint16
		switch addr.Port {
		case c.p.address.Port:
			socket = uint16(quakeIPXSocket)
			c.handleAccept(buf[:n], &c.p.address)
		case c.connectedPort:
			socket = uint16(c.ipxSocket)
			eaten, err := c.rs.receiveFromUpstream(buf[:n])
			if err != nil || eaten {
				// Processed by sharder.
				continue
			}
		default:
			continue
		}
		c.lastRXTime = time.Now()
		if err := c.sendToDownstreamSocket(buf[:n], socket); err != nil {
			// TODO: close connection?
		}
	}
}

type Proxy struct {
	config  Config
	node    network.Node
	conns   map[ipx.HeaderAddr]*connection
	mu      sync.Mutex
	address net.UDPAddr
}

func (p *Proxy) newConnection(ipxAddr *ipx.HeaderAddr) (*connection, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	c := &connection{
		p:             p,
		ipxAddr:       ipxAddr,
		conn:          conn,
		lastRXTime:    time.Now(),
		connectedPort: -1,
		ipxSocket:     connectedIPXSocket,
	}
	c.rs.init(c.sendToUpstream, c.sendToDownstream)
	p.conns[*ipxAddr] = c
	go c.receivePackets()
	return c, nil
}

func (p *Proxy) closeConnection(addr *ipx.HeaderAddr) {
	c, ok := p.conns[*addr]
	if !ok {
		return
	}
	c.closed = true
	delete(p.conns, *addr)
	c.conn.Close()
}

func (p *Proxy) resolveAddress() bool {
	a, err := net.ResolveUDPAddr("udp", p.config.Address)
	if err != nil {
		log.Printf("failed to resolve server address: %v", err)
		return false
	}
	p.address = *a
	return true
}

func (p *Proxy) processPacket(packet *ipx.Packet) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// First connection triggers the server address to be resolved. After
	// all connections time out, we resolve again once a new connection is
	// opened. This handles dynamic DNS addresses where the IP changes.
	// But we don't block on DNS resolution while a game is in progress.
	if len(p.conns) == 0 && !p.resolveAddress() {
		return
	}
	c, ok := p.conns[packet.Header.Src]
	if !ok {
		var err error
		c, err = p.newConnection(&packet.Header.Src)
		if err != nil {
			log.Printf("failed to create new connection to %v: %v", p.address, err)
			return
		}
	}
	c.lastRXTime = time.Now()
	if _, err := c.conn.WriteToUDP(packet.Payload[quakeHeaderBytes:], &p.address); err != nil {
		log.Printf("failed to forward IPX packet to UDP server: %v", err)
		p.closeConnection(&packet.Header.Src)
	}
}

func (p *Proxy) processConnectedPacket(packet *ipx.Packet) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.conns[packet.Header.Src]
	if !ok {
		return
	}
	c.lastRXTime = time.Now()
	msg := packet.Payload[quakeHeaderBytes:]
	eaten, err := c.rs.receiveFromDownstream(msg)
	if err != nil {
		log.Printf("error processing packet from downstream: %v", err)
		p.closeConnection(&packet.Header.Src)
	}
	if eaten {
		// Handled by reliable sharder code.
		return
	}
	if err := c.sendToUpstream(msg); err != nil {
		log.Printf("failed to forward IPX packet to UDP server: %v", err)
		p.closeConnection(&packet.Header.Src)
	}
}

func (p *Proxy) garbageCollect() {
	for {
		time.Sleep(garbageCollectPeriod)
		p.mu.Lock()
		now := time.Now()
		expiredConns := []ipx.HeaderAddr{}
		for addr, c := range p.conns {
			if now.Sub(c.lastRXTime) > p.config.IdleTimeout {
				expiredConns = append(expiredConns, addr)
			}
		}
		for _, addr := range expiredConns {
			p.closeConnection(&addr)
		}
		p.mu.Unlock()
	}
}

func (p *Proxy) Run(ctx context.Context) {
	go p.garbageCollect()
	for {
		packet, err := p.node.ReadPacket(ctx)
		switch {
		case err == io.ErrClosedPipe:
			return
		case err != nil:
			log.Printf("unexpected error reading from node: %v", err)
			return
		}

		if packet.Header.Dest.Socket == quakeIPXSocket {
			p.processPacket(packet)
		} else if packet.Header.Dest.Socket == connectedIPXSocket {
			p.processConnectedPacket(packet)
		}
	}
}

func New(config *Config, node network.Node) *Proxy {
	return &Proxy{
		config: *config,
		node:   node,
		conns:  make(map[ipx.HeaderAddr]*connection),
	}
}
