package phys

import (
	"flag"
	"fmt"
	"github.com/songgao/water"
)

type Flags struct {
	PcapDevice      *string
	EnableTap       *bool
	EthernetFraming *string
}

func RegisterFlags() *Flags {
	f := &Flags{}
	maybeAddPcapDeviceFlag(f)
	f.EnableTap = flag.Bool("enable_tap", false, "Bridge the server to a tap device.")
	f.EthernetFraming = flag.String("ethernet_framing", "auto", `Framing to use when sending Ethernet packets. Valid values are "auto", "802.2", "802.3raw", "snap" and "eth-ii".`)
	return f
}

func (f *Flags) EthernetStream(captureNonIPX bool) (DuplexEthernetStream, error) {
	if *f.EnableTap {
		return NewTap(water.Config{})
	} else if *f.PcapDevice == "" {
		return nil, nil
	}
	return openPcapHandle(f, captureNonIPX)
}

func (f *Flags) makeFramer() (Framer, error) {
	framerName := *f.EthernetFraming
	if framerName == "auto" {
		return &automaticFramer{
			fallback: Framer802_2,
		}, nil
	}
	for _, framer := range allFramers {
		if framerName == framer.Name() {
			return framer, nil
		}
	}
	return nil, fmt.Errorf("unknown Ethernet framing %q", framerName)
}

func (f *Flags) MakePhys(captureNonIPX bool) (*Phys, error) {
	stream, err := f.EthernetStream(captureNonIPX)
	if err != nil {
		return nil, err
	} else if stream != nil {
		framer, err := f.makeFramer()
		if err != nil {
			return nil, err
		}
		return NewPhys(stream, framer), nil
	}
	// Physical capture not enabled.
	return nil, nil
}
