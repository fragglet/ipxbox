// Package phys implements a reader/writer object for reading and writing IPX
// packets from a TAP device.
package phys

import (
	"reflect"
	"time"

	"github.com/google/gopacket"
	"github.com/songgao/packets/ethernet"
	"github.com/songgao/water"
)

var (
	_ = (DuplexEthernetStream)(&tapWrapper{})
)

// tapWrapper implements the DuplexEthernetStream interface by wrapping a
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
func NewTap(cfg water.Config) (*tapWrapper, error) {
	cfg.DeviceType = water.TAP

	ifce, err := water.New(cfg)
	if err != nil {
		return nil, err
	}
	return &tapWrapper{ifce}, nil
}

func openTap(arg string, captureNonIPX bool) (DuplexEthernetStream, error) {
	cfg := water.Config{}
	if arg != "" {
		// We allow the name for the TAP device to be specified. But
		// depending on the OS, the PlatformSpecificParams struct does
		// not always contain a Name field. So we set it through
		// reflection (avoiding convoluted build shenanigans):
		psp := reflect.ValueOf(&cfg.PlatformSpecificParams).Elem()
		if _, ok := psp.Type().FieldByName("Name"); !ok {
			panic("water.PlatformSpecificParams has no name field on this OS")
		}
		psp.FieldByName("Name").SetString(arg)
		// TODO: Allow other parameters to be set, too?
	}
	return NewTap(cfg)
}
