// Package phys implements a reader/writer object for reading and writing IPX
// packets from a TAP device.
package phys

import (
	"time"

	"github.com/google/gopacket"
	"github.com/songgao/packets/ethernet"
	"github.com/songgao/water"
)

var (
	_ = (packetDuplexStream)(&tapWrapper{})
)

// tapWrapper implements the packetDuplexStream interface by wrapping a
// water.Interface.
type tapWrapper struct {
	ifce *water.Interface
}

func (w *tapWrapper) ReadPacketData() ([]byte, gopacket.CaptureInfo, error) {
	var frame ethernet.Frame
	frame.Resize(1500)
	n, err := w.ifce.Read([]byte(frame))
	if err != nil {
		return nil, gopacket.CaptureInfo{}, err
	}
	frame = frame[:n]
	ci := gopacket.CaptureInfo{
		Timestamp:     time.Now(),
		CaptureLength: n,
		Length:        n,
	}
	return frame, ci, nil
}

func (w *tapWrapper) WritePacketData(frame []byte) error {
	_, err := w.ifce.Write(frame)
	return err
}

func (w *tapWrapper) Close() {
	w.ifce.Close()
}

// NewTap creates a new physical IPX interface using a kernel TAP interface.
func NewTap(cfg water.Config, framer Framer) (*Phys, error) {
	cfg.DeviceType = water.TAP

	ifce, err := water.New(cfg)
	if err != nil {
		return nil, err
	}
	return newPhys(&tapWrapper{ifce}, framer), nil
}
