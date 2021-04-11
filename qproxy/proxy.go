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
	quakeHeaderBytes     = 4
)

type Config struct {
	// Address of Quake server.
	Address net.UDPAddr

	// IdleTimeout is the amount of time after which a connection is deleted.
	IdleTimeout time.Duration
}

type connection struct {
	conn       *net.UDPConn
	lastRXTime time.Time
	closed     bool
}

func (c *connection) receivePackets(p *Proxy, ipxAddr *ipx.HeaderAddr) {
	var buf [1500]byte
	for {
		n, err := c.conn.Read(buf[:])
		switch {
		case c.closed:
			return
		case err != nil:
			log.Printf("error receiving UDP packets for connection to %v: %v", c.conn.RemoteAddr(), err)
			return
		}
		c.lastRXTime = time.Now()
		hdr := &ipx.Header{
			Length: uint16(n + ipx.HeaderLength + quakeHeaderBytes),
			Dest:   *ipxAddr,
			Src: ipx.HeaderAddr{
				Addr:   p.node.Address(),
				Socket: quakeIPXSocket,
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
	conn, err := net.DialUDP("udp", nil, &p.config.Address)
	if err != nil {
		return nil, err
	}
	c := &connection{
		conn:       conn,
		lastRXTime: time.Now(),
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
	if _, err := c.conn.Write(pkt[ipx.HeaderLength+quakeHeaderBytes:]); err != nil {
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
		if header.Dest.Socket != quakeIPXSocket {
			continue
		}
		p.processPacket(&header, buf[:n])
	}
}

func New(config *Config, node network.Node) *Proxy {
	return &Proxy{
		config: *config,
		node:   node,
		conns:  make(map[ipx.HeaderAddr]*connection),
	}
}
