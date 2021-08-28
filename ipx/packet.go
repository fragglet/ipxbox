package ipx

import (
	"context"
	"encoding"
	"io"
)

var (
	_ = (encoding.BinaryMarshaler)(&Packet{})
	_ = (encoding.BinaryUnmarshaler)(&Packet{})
)

// Reader defines a common interface implemented by things from which
// IPX packets can be read.
type Reader interface {
	// ReadPacket returns an IPX packet read from this source or an
	// error. If no packet is available yet it will block.
	ReadPacket(context.Context) (*Packet, error)
}

// Writer defines a common interface implemented by things to which
// IPX packets can be written.
type Writer interface {
	// WritePacket writes the given packet, returning an error if the
	// packet could not be written. WritePacket should not block.
	WritePacket(*Packet) error
}

type ReadCloser interface {
	Reader
	io.Closer
}

type WriteCloser interface {
	Writer
	io.Closer
}

type ReadWriteCloser interface {
	Reader
	Writer
	io.Closer
}

// Packet contains an unmarshaled IPX packet containing the header and
// payload bytes.
type Packet struct {
	Header  Header
	Payload []byte
}

func (p *Packet) MarshalBinary() ([]byte, error) {
	result, err := p.Header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	result = append(result, p.Payload...)
	return result, nil
}

func (p *Packet) UnmarshalBinary(packet []byte) error {
	if err := p.Header.UnmarshalBinary(packet); err != nil {
		return err
	}
	p.Payload = append([]byte{}, packet[HeaderLength:]...)
	return nil
}
