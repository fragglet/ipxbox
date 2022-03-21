package ipx

import (
	"context"
	"encoding"
	"errors"
	"io"

	"golang.org/x/sync/errgroup"
)

var (
	_ = (encoding.BinaryMarshaler)(&Packet{})
	_ = (encoding.BinaryUnmarshaler)(&Packet{})
)

// Reader defines a common interface implemented by things from which
// IPX packets can be read.
type Reader interface {
	// ReadPacket returns an IPX packet read from this source or an
	// error. If no packet is available yet it will block. An error
	// of io.EOF indicates that no more packets are available to read.
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

type ReadWriter interface {
	Reader
	Writer
}

type ReadWriteCloser interface {
	ReadWriter
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

// CopyPackets copies packets from in to out until an error occurs whil
// reading or the context is cancelled. If the input returns EOF then
// CopyPackets returns nil to indicate copying completed successfully.
func CopyPackets(ctx context.Context, in Reader, out Writer) error {
	for {
		packet, err := in.ReadPacket(ctx)
		if errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return err
		}
		out.WritePacket(packet)
	}
}

func DuplexCopyPackets(ctx context.Context, x, y ReadWriter) error {
	eg, egctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return CopyPackets(egctx, x, y)
	})
	eg.Go(func() error {
		return CopyPackets(egctx, y, x)
	})
	return eg.Wait()
}
