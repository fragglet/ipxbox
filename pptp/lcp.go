package pptp

import (
	"encoding/binary"
	"errors"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

var (
	LCPMessageTooShort = errors.New("LCP message too short")
)

var LayerTypeLCP = gopacket.RegisterLayerType(1818, gopacket.LayerTypeMetadata{
	Name:    "LCP",
	Decoder: gopacket.DecodeFunc(decodeLCP),
})

// TODO: Implement SerializeTo and make this SerializableLayer.
var _ = gopacket.Layer(&LCP{})

type LCPOptionType uint8

// TODO: constants for common option types

type LCPOption struct {
	Type LCPOptionType
	Data []byte
}

type LCPMessageType uint8

const (
	LCPConfigureRequest LCPMessageType = iota + 1
	LCPConfigureAck
	LCPConfigureNak
	LCPConfigureReject
	LCPTerminateRequest
	LCPTerminateAck
	LCPCodeReject
	LCPProtocolReject
	LCPEchoRequest
	LCPEchoReply
	LCPDiscardRequest
)

// LCP is a gopacket layer for the Link Control Protocol.
type LCP struct {
	layers.BaseLayer
	Type       LCPMessageType
	Identifier uint8
	Options    []LCPOption
}

func (l *LCP) LayerType() gopacket.LayerType {
	return LayerTypeLCP
}

func decodeOptions(data []byte) ([]LCPOption, error) {
	result := []LCPOption{}
	for len(data) > 0 {
		if len(data) < 3 {
			return nil, LCPMessageTooShort
		}
		optType := LCPOptionType(data[0])
		optLen := binary.BigEndian.Uint16(data[1:3])
		if int(optLen) > len(data) {
			return nil, LCPMessageTooShort
		}
		result = append(result, LCPOption{
			Type: optType,
			Data: data[3:optLen],
		})
		data = data[optLen:]
	}
	return result, nil
}

func decodeLCP(data []byte, p gopacket.PacketBuilder) error {
	lcp := &LCP{}
	if len(data) < 4 {
		return LCPMessageTooShort
	}
	lcp.Type = LCPMessageType(data[0])
	lcp.Identifier = data[1]
	lenField := binary.BigEndian.Uint16(data[2:4])
	if int(lenField) > len(data) {
		return LCPMessageTooShort
	}

	var err error
	switch lcp.Type {
	case LCPConfigureRequest, LCPConfigureAck, LCPConfigureNak, LCPConfigureReject:
		lcp.Options, err = decodeOptions(data[4:])
		if err != nil {
			return err
		}
		// TODO: Other message types
	}
	lcp.Contents = data
	lcp.Payload = nil
	p.AddLayer(lcp)
	return nil
}
