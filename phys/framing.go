package phys

import (
	"net"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

type Framer interface {
	Frame(dest net.HardwareAddr, packet *ipx.Packet) ([]gopacket.SerializableLayer, error)
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
)

// GetIPXPayload parses the layers in the given packet to locate and extract
// an IPX payload.
func GetIPXPayload(pkt gopacket.Packet) ([]byte, bool) {
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
	switch eth.EthernetType {
	case etherTypeIPX:
		// ETHERNET_II framing type.
		return eth.LayerPayload(), true
	case layers.EthernetTypeLLC:
		break
	default:
		return nil, false
	}

	if len(nextLayers) < 1 {
		return nil, false
	}
	llc, ok := nextLayers[0].(*layers.LLC)
	if !ok {
		return nil, false
	}
	llcBytes := llc.LayerContents()
	switch {
	case llc.DSAP == lsapNovell && llc.SSAP == lsapNovell:
		// 802.2 framing type.
		// https://en.wikipedia.org/wiki/IEEE_802.2
		return llc.LayerPayload(), true
	case llc.DSAP == lsapSNAP && llc.SSAP == lsapSNAP:
		// SNAP header.
		if len(nextLayers) < 2 {
			return nil, false
		}
		snap, ok := nextLayers[1].(*layers.SNAP)
		if !ok || snap.Type != etherTypeIPX {
			return nil, false
		}
		return snap.LayerPayload(), true
	case llcBytes[0] == 0xff && llcBytes[1] == 0xff:
		// Novell "raw" 802.3:
		// https://en.wikipedia.org/wiki/Ethernet_frame#Novell_raw_IEEE_802.3
		// "This does not conform to the IEEE 802.3 standard, but
		// since IPX always has FF as the first two octets" it can be
		// interpreted correctly.
		return eth.LayerPayload(), true
	default:
		return nil, false
	}
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
