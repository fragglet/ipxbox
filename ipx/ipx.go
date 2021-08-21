// Package ipx implements common types for IPX header encoding and
// decoding.
package ipx

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"fmt"
	"net"
)

// Addr represents an IPX address (MAC address).
type Addr [6]byte

// HeaderAddr represents a full IPX address and socket number.
type HeaderAddr struct {
	Network [4]byte
	Addr    Addr
	Socket  uint16
}

// Header represents an IPX header.
type Header struct {
	Checksum     uint16
	Length       uint16
	TransControl byte
	PacketType   byte
	Dest, Src    HeaderAddr
}

var (
	// Check that the Address type implements the net.Addr interface.
	_ = (net.Addr)(&AddrNull)

	// Check the BinaryMarshaler/Unmarshaler interfaces are implemented.
	_ = (encoding.BinaryMarshaler)(&HeaderAddr{})
	_ = (encoding.BinaryUnmarshaler)(&HeaderAddr{})
	_ = (encoding.BinaryMarshaler)(&Header{})
	_ = (encoding.BinaryUnmarshaler)(&Header{})

	AddrNull      = Addr([6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	AddrBroadcast = Addr([6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})

	HeaderLength           = 30
	minHeaderAddressLength = 12
)

func (a Addr) Network() string {
	return "dosbox-ipx"
}

func (a Addr) String() string {
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", a[0], a[1], a[2], a[3], a[4], a[5])
}

// UnmarshalBinary decodes an IPX header address from a slice of bytes.
func (a *HeaderAddr) UnmarshalBinary(data []byte) error {
	if len(data) < minHeaderAddressLength {
		return fmt.Errorf("Header address too short to decode: %d < %d", len(data), minHeaderAddressLength)
	}
	copy(a.Network[0:], data[0:4])
	copy(a.Addr[0:], data[4:10])
	a.Socket = (uint16(data[10]) << 8) | uint16(data[11])
	return nil
}

// MarshalBinary populates a slice of bytes from an IPX header address.
func (a *HeaderAddr) MarshalBinary() ([]byte, error) {
	result := make([]byte, 12)
	copy(result[0:4], a.Network[0:4])
	copy(result[4:10], a.Addr[0:])
	result[10] = byte(a.Socket >> 8)
	result[11] = byte(a.Socket & 0xff)
	return result, nil
}

// UnmarshalBinary decodes an IPX header from a slice of bytes.
func (h *Header) UnmarshalBinary(packet []byte) error {
	if len(packet) < HeaderLength {
		return fmt.Errorf("IPX header too short to decode: %d < %d", len(packet), HeaderLength)
	}

	h.Checksum = binary.BigEndian.Uint16(packet[0:2])
	h.Length = binary.BigEndian.Uint16(packet[2:4])
	h.TransControl = packet[4]
	h.PacketType = packet[5]

	if err := h.Dest.UnmarshalBinary(packet[6:18]); err != nil {
		return err
	}
	return h.Src.UnmarshalBinary(packet[18:30])
}

// MarshalBinary populates a slice of bytes from an IPX header.
func (h *Header) MarshalBinary() ([]byte, error) {
	result := []byte{
		byte(h.Checksum >> 8),
		byte(h.Checksum & 0xff),
		byte(h.Length >> 8),
		byte(h.Length & 0xff),
		h.TransControl,
		h.PacketType,
	}
	dest, err := h.Dest.MarshalBinary()
	if err != nil {
		return nil, err
	}
	src, err := h.Src.MarshalBinary()
	if err != nil {
		return nil, err
	}
	result = append(result, dest...)
	result = append(result, src...)
	return result, nil
}

func (h *Header) IsRegistrationPacket() bool {
	return h.Dest.Socket == 2 && bytes.Equal(h.Dest.Addr[0:], AddrNull[:])
}

func (h *Header) IsBroadcast() bool {
	return bytes.Equal(h.Dest.Addr[0:], AddrBroadcast[:])
}
