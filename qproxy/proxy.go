// Package qproxy implements a proxy client that makes Quake UDP servers
// available on an IPX network.
package qproxy

import (
	"bytes"
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
	acceptHeaderLen = 9
)

var (
	acceptHeaderBytes = []byte{
		0x80,  // NETFLAG_CTL
		0x00,
		0x00, 0x09, // length=9
		0x81,  // CCREP_ACCEPT
	}
)

type Config struct {
	// Address of Quake server.
	Address net.UDPAddr

	// IdleTimeout is the amount of time after which a connection is deleted.
	IdleTimeout time.Duration
}

type connection struct {
	conn          *net.UDPConn
	lastRXTime    time.Time
	connectedPort int
	closed        bool
}

// handleAccept checks if a packet received from the main server port is a
// CCREP_ACCEPT packet, and if so, reads the connected port number from the
// packet, then replaces it with connectedIPXSocket.
func (c *connection) handleAccept(packet []byte) {
	if len(packet) != acceptHeaderLen {
		return
	}
	if !bytes.Equal(acceptHeaderBytes, packet[:len(acceptHeaderBytes)]) {
		return
	}
	// We have a legitimate looking CCREP_ACCEPT packet.
	// The server has indicated the port number assigned for this
	// connection as part of the packet.
	c.connectedPort = (int(packet[5]) << 8) | int(packet[6])
	// Before forwarding onto the IPX network, we must replace the UDP
	// socket number with the connected IPX port number.
	packet[5] = byte((connectedIPXSocket >> 8) & 0xff)
	packet[6] = byte(connectedIPXSocket & 0xff)
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
		if !addr.IP.Equal(p.config.Address.IP) {
			continue
		}
		// Packet must come from either the server's main port or from
		// the port assigned to this connection. Map this into the IPX
		// socket number for the source address.
		socket := uint16(connectedIPXSocket)
		if addr.Port == p.config.Address.Port {
			socket = uint16(quakeIPXSocket)
			c.handleAccept(buf[:n])
		} else if addr.Port != c.connectedPort {
			continue
		}
		c.lastRXTime = time.Now()
		hdr := &ipx.Header{
			Length: uint16(n + ipx.HeaderLength + quakeHeaderBytes),
			Dest:   *ipxAddr,
			Src: ipx.HeaderAddr{
				Addr:   p.node.Address(),
				Socket: socket,
			},
		}
		pktBytes, err := hdr.MarshalBinary()
		if err != nil {
			log.Printf("error marshalling IPX packet: %v", err)
			continue
		}
		zeroBytes := [quakeHeaderBytes]byte{}
		pktBytes = append(pktBytes, zeroBytes[:]...)
		pktBytes = append(pktBytes, buf[:n]...)
		if _, err := p.node.Write(pktBytes); err != nil {
			// TODO: close connection?
			return
		}
	}
}

type Proxy struct {
	config Config
	node   network.Node
	conns  map[ipx.HeaderAddr]*connection
	mu     sync.Mutex
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

func (p *Proxy) processPacket(hdr *ipx.Header, pkt []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.conns[hdr.Src]
	if !ok {
		var err error
		c, err = p.newConnection(&hdr.Src)
		if err != nil {
			log.Printf("failed to create new connection to %v: %v", p.config.Address, err)
			return
		}
	}
	c.lastRXTime = time.Now()
	if _, err := c.conn.WriteToUDP(pkt[ipx.HeaderLength+quakeHeaderBytes:], &p.config.Address); err != nil {
		log.Printf("failed to forward IPX packet to UDP server: %v", err)
		p.closeConnection(&hdr.Src)
	}
}

func (p *Proxy) processConnectedPacket(hdr *ipx.Header, pkt []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.conns[hdr.Src]
	if !ok || c.connectedPort < 0{
		return
	}
	destAddress := &net.UDPAddr{
		IP:   p.config.Address.IP,
		Port: c.connectedPort,
	}
	c.lastRXTime = time.Now()
	if _, err := c.conn.WriteToUDP(pkt[ipx.HeaderLength+quakeHeaderBytes:], destAddress); err != nil {
		log.Printf("failed to forward IPX packet to UDP server: %v", err)
		p.closeConnection(&hdr.Src)
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
	var buf [1500]byte
	for {
		n, err := p.node.Read(buf[:])
		switch {
		case err == io.EOF:
			return
		case err != nil:
			// Other errors are ignored.
			continue
		}

		var header ipx.Header
		if err := header.UnmarshalBinary(buf[:n]); err != nil {
			continue
		}
		if header.Dest.Socket == quakeIPXSocket {
			p.processPacket(&header, buf[:n])
		} else if header.Dest.Socket == connectedIPXSocket {
			p.processConnectedPacket(&header, buf[:n])
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
