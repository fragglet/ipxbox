// Package phys implements a reader/writer object for reading and writing IPX
// packets from a TAP device.
package phys

import (
	"io"
	"net"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/songgao/packets/ethernet"
	"github.com/songgao/water"
)

type Phys struct {
	ifce *water.Interface
}

var (
	_ = (io.ReadWriteCloser)(&Phys{})
)

// New creates a new physical IPX interface.
func New(cfg water.Config) (*Phys, error) {
	cfg.DeviceType = water.TAP

	ifce, err := water.New(cfg)
	if err != nil {
		return nil, err
	}
	return &Phys{ifce}, nil
}

// Read implements the io.Reader interface, and will block until an IPX packet
// is received from the TAP device.
func (p *Phys) Read(result []byte) (int, error) {
	var frame ethernet.Frame
	for {
		frame.Resize(1500)
		n, err := p.ifce.Read([]byte(frame))
		if err != nil {
			return 0, err
		}
		frame = frame[:n]
		if frame.Ethertype() == ethernet.IPX1 {
			break
		}
	}
	// We got an IPX frame
	pl := frame.Payload()
	cnt := len(pl)
	if len(result) < cnt {
		cnt = len(result)
	}
	copy(result[0:cnt], pl[0:cnt])
	return cnt, nil
}

// Write writes an ethernet frame to the TAP interface containing the given IPX
// packet as payload.
func (p *Phys) Write(packet []byte) (int, error) {
	var hdr ipx.Header
	if err := hdr.UnmarshalBinary(packet); err != nil {
		return 0, err
	}
	var frame ethernet.Frame
	dst := net.HardwareAddr(hdr.Dest.Addr[:])
	src := net.HardwareAddr(hdr.Src.Addr[:])
	frame.Prepare(dst, src, ethernet.NotTagged, ethernet.IPX1, len(packet))
	copy(frame.Payload(), packet)
	return p.ifce.Write(frame)
}

func (p *Phys) Close() error {
	return p.ifce.Close()
}
