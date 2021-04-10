// Package qproxy implements a proxy client that makes Quake UDP servers
// available on an IPX network.
package qproxy

import (
	"io"
	"net"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
)

const (
	quakeIPXSocket   = 26000
	quakeHeaderBytes = 4
)

type Config struct {
	// Address of Quake server.
	Address net.UDPAddr
}

type connection struct {
	conn *net.UDPConn
}

func (c *connection) receivePackets(p *Proxy, ipxAddr *ipx.HeaderAddr) {
	var buf [1500]byte
	for {
		n, err := c.conn.Read(buf[:])
		if err != nil {
			// TODO: log
			return
		}
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
			// TODO: log
			continue
		}
		zeroBytes := [quakeHeaderBytes]byte{}
		pktBytes = append(pktBytes, zeroBytes[:]...)
		pktBytes = append(pktBytes, buf[:n]...)
		if _, err := p.node.Write(pktBytes); err != nil {
			// TODO: log
		}
	}
}

type Proxy struct {
	config Config
	node   network.Node
	conns  map[ipx.HeaderAddr]*connection
}

func (p *Proxy) newConnection(ipxAddr *ipx.HeaderAddr) (*connection, error) {
	conn, err := net.DialUDP("udp", nil, &p.config.Address)
	if err != nil {
		return nil, err
	}
	c := &connection{
		conn: conn,
	}
	p.conns[*ipxAddr] = c
	go c.receivePackets(p, ipxAddr)
	return c, nil
}

func (p *Proxy) processPacket(hdr *ipx.Header, pkt []byte) {
	conn, ok := p.conns[hdr.Src]
	if !ok {
		var err error
		conn, err = p.newConnection(&hdr.Src)
		if err != nil {
			// TODO: log
			return
		}
	}
	if _, err := conn.conn.Write(pkt[ipx.HeaderLength+quakeHeaderBytes:]); err != nil {
		// TODO: log
	}
}

func (p *Proxy) Run() {
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
