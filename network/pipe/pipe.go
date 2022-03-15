// Package pipe implements a nonblocking Reader/Writer pair; data can be
// written to the writer end of the pipe and read from the reader end of
// the pipe. This is similar to io.Pipe() with some differences: first,
// we implement the ipx.ReadWriteCloser interface. Second, calls to
// WritePacket() never block. Third, there is an internal buffer of packets
// that have been written but not yet read from the pipe. The size of the
// buffer is configurable. Once the buffer is full, WritePacket() will
// return errors until the reader drains the pipe.
package pipe

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/fragglet/ipxbox/ipx"
)

const (
	// maxBufferedPackets is the number of packets to buffer in a pipe
	// before we start to drop packets. The rationale for this number
	// is as follows: in a peer-to-peer game (Doom, Duke3D...) it is
	// common to send a burst of packets, one to every other node in
	// the game. Therefore we should be able to cope with such bursts
	// up to the maximum number of players we might plausibly see in
	// an IPX game. This seems like a reasonable upper bound.
	maxBufferedPackets = 16
)

var (
	_ = (ipx.ReadWriteCloser)(&pipe{})

	PipeFullError = errors.New("pipe buffer is full")
)

type pipe struct {
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

// WritePacket sends a packet to the channel. This function never blocks. If
// the pipe can hold no more data (eg. the reader has stopped reading) then
// PipeFullError may be returned.
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

// ReadPacket blocks until a packet is received, the pipe is closed or the
// context expires.
func (p *pipe) ReadPacket(ctx context.Context) (*ipx.Packet, error) {
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return nil, io.ErrClosedPipe
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case pkt, ok := <-p.ch:
		if !ok {
			return nil, io.ErrClosedPipe
		}
		return pkt, nil
	}
}

// New returns a new pipe that buffers a number of writes internally.
// This is conceptually similar to io.Pipe(), but for IPX packets.
func New() *pipe {
	p := &pipe{
		ch: make(chan *ipx.Packet, maxBufferedPackets),
	}
	return p
}
