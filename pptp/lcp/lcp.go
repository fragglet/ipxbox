// Package lcp contains a gopacket Layer that implements the PPP Link Control
// Protocol (LCP).
package lcp

import (
	"encoding"
	"encoding/binary"
	"errors"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

const PPPTypeLCP = layers.PPPType(0xc021)

var (
	MessageTooShort = errors.New("LCP message too short")

	LayerTypeLCP = gopacket.RegisterLayerType(1818, gopacket.LayerTypeMetadata{
		Name:    "LCP",
		Decoder: gopacket.DecodeFunc(decodeLCP),
	})
)

// TODO: Implement SerializeTo and make this SerializableLayer.
var _ = gopacket.Layer(&LCP{})

// OptionType identifies an LCP option, at least in the context of the
// particular dialect of LCP being used - the same ID will have different
// meanings in LCP, IPXCP, etc.
type OptionType uint8

var (
	OptionMRU                       = OptionType(1)
	OptionAuthProtocol              = OptionType(3)
	OptionQualityProtocol           = OptionType(4)
	OptionMagicNumber               = OptionType(5)
	OptionProtocolFieldCompression  = OptionType(7)
	OptionAddressControlCompression = OptionType(8)
)

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

// PerTypeData specifies a common interface that is implemented by other types
// that represent per-message-type data.
type PerTypeData interface {
	encoding.BinaryUnmarshaler
}

// ConfigureData contains the data that is specific to Configure-* messages.
type ConfigureData struct {
	Options []Option
}

func (d *ConfigureData) UnmarshalBinary(data []byte) error {
	result := []Option{}
	for len(data) > 0 {
		if len(data) < 3 {
			return MessageTooShort
		}
		optType := OptionType(data[0])
		optLen := binary.BigEndian.Uint16(data[1:3])
		if int(optLen) > len(data) {
			return MessageTooShort
		}
		result = append(result, Option{
			Type: optType,
			Data: data[3:optLen],
		})
		data = data[optLen:]
	}
	d.Options = result
	return nil
}

// TerminateData contains the data that is specific to Terminate-* messages.
type TerminateData struct {
	Data []byte
}

func (d *TerminateData) UnmarshalBinary(data []byte) error {
	d.Data = data
	return nil
}

// EchoData contains the data that is specific to echo-* messages.
type EchoData struct {
	MagicNumber uint32
	Data        []byte
}

func (d *EchoData) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return MessageTooShort
	}
	d.MagicNumber = binary.BigEndian.Uint32(data[:4])
	d.Data = data[4:]
	return nil
}

// BaseLayer represents the basic LCP layer used by LCP proper
// and other dialects that reuse the same format.
type BaseLayer struct {
	layers.BaseLayer
	PPPType    layers.PPPType
	Type       MessageType
	Identifier uint8
	Data       PerTypeData
}

func (l *BaseLayer) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return MessageTooShort
	}
	l.Type = MessageType(data[0])
	l.Identifier = data[1]
	lenField := binary.BigEndian.Uint16(data[2:4])
	if int(lenField) > len(data) {
		return MessageTooShort
	}

	switch l.Type {
	case ConfigureRequest, ConfigureAck, ConfigureNak, ConfigureReject:
		l.Data = &ConfigureData{}
	case TerminateRequest, TerminateAck:
		l.Data = &TerminateData{}
	case EchoRequest, EchoReply, DiscardRequest:
		l.Data = &EchoData{}
		// TODO: Other message types.
	}
	if l.Data != nil {
		if err := l.Data.UnmarshalBinary(data[4:]); err != nil {
			return err
		}
	}
	l.Contents = data
	l.Payload = nil
	return nil
}

// LCP is a gopacket layer for the Link Control Protocol.
type LCP struct {
	BaseLayer
}

func (l *LCP) LayerType() gopacket.LayerType {
	return LayerTypeLCP
}

func decodeLCP(data []byte, p gopacket.PacketBuilder) error {
	lcp := &LCP{}
	lcp.PPPType = PPPTypeLCP
	if err := lcp.UnmarshalBinary(data); err != nil {
		return err
	}
	p.AddLayer(lcp)
	return nil
}

func init() {
	// Hook in our layers on startup to the PPP layer, so that we will get
	// them decoded automatically if found inside PPP frames.
	layers.PPPTypeMetadata[PPPTypeLCP] = layers.EnumMetadata{
		DecodeWith: gopacket.DecodeFunc(decodeLCP),
		Name: "LCP",
	}
	layers.PPPTypeMetadata[PPPTypeIPXCP] = layers.EnumMetadata{
		DecodeWith: gopacket.DecodeFunc(decodeIPXCP),
		Name: "IPXCP",
	}
}
