package ipx

import (
	"encoding"
	"io"
)

var (
	_ = (encoding.BinaryMarshaler)(&Packet{})
	_ = (encoding.BinaryUnmarshaler)(&Packet{})
	_ = (io.Reader)(&ReaderShim{})
	_ = (io.Writer)(&WriterShim{})
)

// Reader defines a common interface implemented by things from which
// IPX packets can be read.
type Reader interface {
	// ReadPacket returns an IPX packet read from this source or an
	// error. If no packet is available yet it will block.
	ReadPacket() (*Packet, error)
}

// Writer defines a common interface implemented by things to which
// IPX packets can be written.
type Writer interface {
	// WritePacket writes the given packet, returning an error if the
	// packet could not be written. WritePacket should not block.
	WritePacket(*Packet) error
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

// ReaderShim implemenets an io.Reader based on an ipx.Reader.
type ReaderShim struct {
	Reader
}

func (r *ReaderShim) Read(p []byte) (n int, err error) {
	pkt, err := r.Reader.ReadPacket()
	if err != nil {
		return 0, err
	}
	pktBytes, err := pkt.MarshalBinary()
	if err != nil {
		return 0, err
	}
	cnt := len(pktBytes)
	if cnt > len(p) {
		cnt = len(p)
	}
	copy(p[:cnt], pktBytes[:cnt])
	return cnt, nil
}

// WriterShim implements an io.Writer based on an ipx.Writer.
type WriterShim struct {
	Writer
}

func (w *WriterShim) Write(p []byte) (n int, err error) {
	packet := &Packet{}
	if err := packet.UnmarshalBinary(p); err != nil {
		return 0, err
	}
	if err := w.Writer.WritePacket(packet); err != nil {
		return 0, err
	}
	return len(p), nil
}
