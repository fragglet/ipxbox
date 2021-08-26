// Package qproxy implements a proxy client that makes Quake UDP servers
// available on an IPX network.
package qproxy

import (
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
	conn          *net.UDPConn
	lastRXTime    time.Time
	connectedPort int
	ipxSocket     int
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

func (c *connection) receivePackets(p *Proxy, ipxAddr *ipx.HeaderAddr) {
	var buf [1500]byte
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
		if !addr.IP.Equal(p.address.IP) {
			continue
		}
		// Packet must come from either the server's main port or from
		// the port assigned to this connection. Map this into the IPX
		// socket number for the source address.
		socket := uint16(c.ipxSocket)
		if addr.Port == p.address.Port {
			socket = uint16(quakeIPXSocket)
			c.handleAccept(buf[:n], &p.address)
		} else if addr.Port != c.connectedPort {
			continue
		}
		c.lastRXTime = time.Now()
		zeroBytes := [quakeHeaderBytes]byte{}
		pktBytes := append([]byte{}, zeroBytes[:]...)
		pktBytes = append(pktBytes, buf[:n]...)
		packet := ipx.Packet{
			Header: ipx.Header{
				Length: uint16(n + ipx.HeaderLength + quakeHeaderBytes),
				Dest:   *ipxAddr,
				Src: ipx.HeaderAddr{
					Addr:   network.NodeAddress(p.node),
					Socket: socket,
				},
			},
			Payload: pktBytes,
		}
		if err := p.node.WritePacket(&packet); err != nil {
			// TODO: close connection?
			return
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
		conn:          conn,
		lastRXTime:    time.Now(),
		connectedPort: -1,
		ipxSocket:     connectedIPXSocket,
	}
	p.conns[*ipxAddr] = c
	go c.receivePackets(p, ipxAddr)
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
	if !ok || c.connectedPort < 0 {
		return
	}
	destAddress := &net.UDPAddr{
		IP:   p.address.IP,
		Port: c.connectedPort,
	}
	c.lastRXTime = time.Now()
	if _, err := c.conn.WriteToUDP(packet.Payload[quakeHeaderBytes:], destAddress); err != nil {
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

func (p *Proxy) Run() {
	go p.garbageCollect()
	for {
		packet, err := p.node.ReadPacket()
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
