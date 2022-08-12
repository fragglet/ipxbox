package phys

import (
	"net"
	"sync"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

type Framer interface {
	Frame(dest net.HardwareAddr, packet *ipx.Packet) ([]gopacket.SerializableLayer, error)
	Unframe(eth *layers.Ethernet, layers []gopacket.Layer) ([]byte, bool)
	Name() string
}

const (
	etherTypeIPX = layers.EthernetType(0x8137)

	lsapNovell = 0xe0
	lsapSNAP   = 0xaa
)

var (
	Framer802_2      = framer802_2{}
	Framer802_3Raw   = framer802_3Raw{}
	FramerSNAP       = framerSNAP{}
	FramerEthernetII = framerEthernetII{}

	allFramers = []Framer{Framer802_2, Framer802_3Raw, FramerEthernetII, FramerSNAP}
)

// Unframe parses the layers in the given packet to locate and extract
// an IPX payload.
func Unframe(pkt gopacket.Packet, framer Framer) ([]byte, bool) {
	var (
		eth        *layers.Ethernet
		nextLayers []gopacket.Layer
	)
	ls := pkt.Layers()
	for idx, l := range ls {
		var ok bool
		eth, ok = l.(*layers.Ethernet)
		if ok {
			nextLayers = ls[idx+1:]
			break
		}
	}

	if eth == nil {
		return nil, false
	}
	return framer.Unframe(eth, nextLayers)
}

type framer802_2 struct{}

func (framer802_2) Frame(dest net.HardwareAddr, packet *ipx.Packet) ([]gopacket.SerializableLayer, error) {
	payload, err := packet.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return []gopacket.SerializableLayer{
		&layers.Ethernet{
			SrcMAC:       net.HardwareAddr(packet.Header.Src.Addr[:]),
			DstMAC:       dest,
			EthernetType: layers.EthernetTypeLLC,
			Length:       uint16(len(payload) + 3),
		},
		&layers.LLC{
			DSAP:    lsapNovell,
			SSAP:    lsapNovell,
			Control: 3,
		},
		gopacket.Payload(payload),
	}, nil
}

func (framer802_2) Unframe(eth *layers.Ethernet, nextLayers []gopacket.Layer) ([]byte, bool) {
	if eth.EthernetType != layers.EthernetTypeLLC {
		return nil, false
	}
	if len(nextLayers) < 1 {
		return nil, false
	}
	llc, ok := nextLayers[0].(*layers.LLC)
	if !ok || llc.DSAP != lsapNovell || llc.SSAP != lsapNovell {
		return nil, false
	}
	// 802.2 framing type.
	// https://en.wikipedia.org/wiki/IEEE_802.2
	return llc.LayerPayload(), true
}

func (framer802_2) Name() string { return "802.2" }

type framer802_3Raw struct{}

func (framer802_3Raw) Frame(dest net.HardwareAddr, packet *ipx.Packet) ([]gopacket.SerializableLayer, error) {
	payload, err := packet.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return []gopacket.SerializableLayer{
		&layers.Ethernet{
			SrcMAC: net.HardwareAddr(packet.Header.Src.Addr[:]),
			DstMAC: dest,
			Length: uint16(len(payload)),
		},
		gopacket.Payload(payload),
	}, nil
}

func (framer802_3Raw) Unframe(eth *layers.Ethernet, nextLayers []gopacket.Layer) ([]byte, bool) {
	if eth.EthernetType != layers.EthernetTypeLLC {
		return nil, false
	}
	if len(nextLayers) < 1 {
		return nil, false
	}
	llc, ok := nextLayers[0].(*layers.LLC)
	if !ok || llc.DSAP != lsapNovell || llc.SSAP != lsapNovell {
		return nil, false
	}
	llcBytes := llc.LayerContents()
	if llcBytes[0] != 0xff || llcBytes[1] != 0xff {
		return nil, false
	}
	// Novell "raw" 802.3:
	// https://en.wikipedia.org/wiki/Ethernet_frame#Novell_raw_IEEE_802.3
	// "This does not conform to the IEEE 802.3 standard, but
	// since IPX always has FF as the first two octets" it can be
	// interpreted correctly.
	return eth.LayerPayload(), true
}

func (framer802_3Raw) Name() string { return "802.3raw" }

type framerSNAP struct{}

func (framerSNAP) Frame(dest net.HardwareAddr, packet *ipx.Packet) ([]gopacket.SerializableLayer, error) {
	payload, err := packet.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return []gopacket.SerializableLayer{
		&layers.Ethernet{
			SrcMAC:       net.HardwareAddr(packet.Header.Src.Addr[:]),
			DstMAC:       dest,
			EthernetType: layers.EthernetTypeLLC,
			Length:       uint16(len(payload) + 8),
		},
		&layers.LLC{
			DSAP:    lsapSNAP,
			SSAP:    lsapSNAP,
			Control: 3,
		},
		&layers.SNAP{
			Type:               etherTypeIPX,
			OrganizationalCode: []byte{0, 0, 0},
		},
		gopacket.Payload(payload),
	}, nil
}

func (framerSNAP) Unframe(eth *layers.Ethernet, nextLayers []gopacket.Layer) ([]byte, bool) {
	if eth.EthernetType != layers.EthernetTypeLLC {
		return nil, false
	}
	if len(nextLayers) < 2 {
		return nil, false
	}
	llc, ok := nextLayers[0].(*layers.LLC)
	if !ok || llc.DSAP != lsapSNAP || llc.SSAP != lsapSNAP {
		return nil, false
	}
	snap, ok := nextLayers[1].(*layers.SNAP)
	if !ok || snap.Type != etherTypeIPX {
		return nil, false
	}
	return snap.LayerPayload(), true
}

func (framerSNAP) Name() string { return "snap" }

type framerEthernetII struct{}

func (framerEthernetII) Frame(dest net.HardwareAddr, packet *ipx.Packet) ([]gopacket.SerializableLayer, error) {
	payload, err := packet.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return []gopacket.SerializableLayer{
		&layers.Ethernet{
			SrcMAC:       net.HardwareAddr(packet.Header.Src.Addr[:]),
			DstMAC:       dest,
			EthernetType: etherTypeIPX,
		},
		gopacket.Payload(payload),
	}, nil
}

func (framerEthernetII) Unframe(eth *layers.Ethernet, nextLayers []gopacket.Layer) ([]byte, bool) {
	if eth.EthernetType != etherTypeIPX {
		return nil, false
	}
	// ETHERNET_II framing type.
	return eth.LayerPayload(), true
}

func (framerEthernetII) Name() string { return "eth-ii" }

// automaticFramer picks a framer based on the first IPX packet it receives.
type automaticFramer struct {
	framer, fallback Framer
	mu               sync.RWMutex
}

func (f *automaticFramer) Frame(dest net.HardwareAddr, packet *ipx.Packet) ([]gopacket.SerializableLayer, error) {
	f.mu.RLock()
	framer := f.framer
	if framer == nil {
		framer = f.fallback
	}
	f.mu.RUnlock()
	return framer.Frame(dest, packet)
}

func (f *automaticFramer) detectedFramer(detected Framer, payload []byte) {
	f.mu.RLock()
	framer := f.framer
	f.mu.RUnlock()
	if framer == nil {
		// We received a packet and know what framing it used. But
		// before we use this as our autodetected framing, make sure
		// that this isn't a looped-back packet and really came from
		// another machine on the network.
		ipxpkt := &ipx.Packet{}
		if err := ipxpkt.UnmarshalBinary(payload); err != nil {
			return
		}
		if ipxpkt.Header.TransControl != loopbackDetectValue {
			// TODO: Write detected.Name() to log file
			f.mu.Lock()
			f.framer = detected
			f.mu.Unlock()
		}
	}
}

func (f *automaticFramer) Unframe(eth *layers.Ethernet, nextLayers []gopacket.Layer) ([]byte, bool) {
	for _, framer := range allFramers {
		result, ok := framer.Unframe(eth, nextLayers)
		if ok {
			f.detectedFramer(framer, result)
			return result, true
		}
	}
	return nil, false
}

func (f *automaticFramer) Name() string { return "auto" }
