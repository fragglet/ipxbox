// Package pipe implements a nonblocking Reader/Writer pair; data can be
// written to the writer end of the pipe and read from the reader end of
// the pipe, and the io.ReadCloser/io.WriteCloser interfaces are
// implemented. This is similar to io.Pipe() with two differences:
// firstly, calls to Write() never block. Secondly, there is an internal
// buffer of byte slices that have been written but not yet read from
// the pipe. The size of the buffer is configurable. Once the buffer is
// full, Write() will start to return errors.
package pipe

import (
	"errors"
	"io"
	"sync"

	"github.com/fragglet/ipxbox/ipx"
)

var (
	PipeFullError = errors.New("pipe buffer is full")
)

type pipe struct {
	ipx.ReaderShim
	ipx.WriterShim
	ch     chan *ipx.Packet
	closed bool
	mu     sync.Mutex
}

func (p *pipe) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.closed = true
		close(p.ch)
	}
	return nil
}

// Write sends a packet to the channel. This function never blocks. If
// the pipe can hold no more data (eg. the reader has stopped reading) then
// PipeFullError may be returned. This function will return len(data) even
// if the reader was not able to read all those bytes.
func (p *pipe) WritePacket(pkt *ipx.Packet) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Sending to a closed channel will result in a runtime panic.
	// Instead, if the pipe has been closed, return an error.
	if p.closed {
		return io.ErrClosedPipe
	}
	select {
	case p.ch <- pkt:
		return nil
	default:
		return PipeFullError
	}
}

// ReadPacket blocks until data can be read into the provided buffer or until
// the pipe is closed.
func (p *pipe) ReadPacket() (*ipx.Packet, error) {
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return nil, io.ErrClosedPipe
	}
	pkt, ok := <-p.ch
	if !ok {
		return nil, io.ErrClosedPipe
	}
	return pkt, nil
}

// New returns a new pipe that buffers `size` number of writes internally.
// This is conceptually similar to io.Pipe(), except for the differences
// listed in the package description, and the fact that we return only a
// single thing that implements both Reader and Writer.
func New(size int) *pipe {
	p := &pipe{
		ch: make(chan *ipx.Packet, size),
	}
	p.ReaderShim.Reader = p
	p.WriterShim.Writer = p
	return p
}
