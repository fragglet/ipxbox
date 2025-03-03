package ipxpkt

import (
	"encoding"
	"fmt"
)

const (
	HeaderLength = 4
)

var (
	// Check the BinaryMarshaler/Unmarshaler interfaces are implemented.
	_ = (encoding.BinaryMarshaler)(&Header{})
	_ = (encoding.BinaryUnmarshaler)(&Header{})
)

// Header represents the fragmentation header for an ipxpkt fragment.
type Header struct {
	Fragment, NumFragments uint8
	PacketID               uint16
}

// MarshalBinary populates a slice of bytes from an ipxpkt header.
func (h *Header) MarshalBinary() ([]byte, error) {
	return []byte{
		h.Fragment,
		h.NumFragments,
		byte(h.PacketID & 0xff),
		byte((h.PacketID >> 8) & 0xff),
	}, nil
}

// UnmarshalBinary decodes an ipxpkt header from a slice of bytes.
func (h *Header) UnmarshalBinary(packet []byte) error {
	if len(packet) < HeaderLength {
		return fmt.Errorf("packet too short to contain an ipxpkt header: %d < %d", len(packet), HeaderLength)
	}
	h.Fragment = packet[0]
	h.NumFragments = packet[1]
	h.PacketID = uint16(packet[2]) | uint16(packet[3]<<8)
	if h.Fragment < 1 || h.NumFragments < 1 || h.Fragment > h.NumFragments {
		return fmt.Errorf("bad ipxpkt header violates invariants: %+v", h)
	}
	return nil
}
