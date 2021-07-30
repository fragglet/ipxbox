// Package lcp contains a gopacket Layer that implements the PPP Link Control
// Protocol (LCP).
package lcp

import (
	"encoding/binary"
	"errors"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

var (
	MessageTooShort = errors.New("LCP message too short")
)

var LayerTypeLCP = gopacket.RegisterLayerType(1818, gopacket.LayerTypeMetadata{
	Name:    "LCP",
	Decoder: gopacket.DecodeFunc(decodeLCP),
})

// TODO: Implement SerializeTo and make this SerializableLayer.
var _ = gopacket.Layer(&LCP{})

type OptionType uint8

// TODO: constants for common option types

type Option struct {
	Type OptionType
	Data []byte
}

type MessageType uint8

const (
	ConfigureRequest MessageType = iota + 1
	ConfigureAck
	ConfigureNak
	ConfigureReject
	TerminateRequest
	TerminateAck
	CodeReject
	ProtocolReject
	EchoRequest
	EchoReply
	DiscardRequest
)

// LCP is a gopacket layer for the Link Control Protocol.
type LCP struct {
	layers.BaseLayer
	Type       MessageType
	Identifier uint8
	Options    []Option
}

func (l *LCP) LayerType() gopacket.LayerType {
	return LayerTypeLCP
}

func decodeOptions(data []byte) ([]Option, error) {
	result := []Option{}
	for len(data) > 0 {
		if len(data) < 3 {
			return nil, MessageTooShort
		}
		optType := OptionType(data[0])
		optLen := binary.BigEndian.Uint16(data[1:3])
		if int(optLen) > len(data) {
			return nil, MessageTooShort
		}
		result = append(result, Option{
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
		return MessageTooShort
	}
	lcp.Type = MessageType(data[0])
	lcp.Identifier = data[1]
	lenField := binary.BigEndian.Uint16(data[2:4])
	if int(lenField) > len(data) {
		return MessageTooShort
	}

	var err error
	switch lcp.Type {
	case ConfigureRequest, ConfigureAck, ConfigureNak, ConfigureReject:
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
