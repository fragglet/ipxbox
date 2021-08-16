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
)

var (
	PipeFullError = errors.New("pipe buffer is full")
)

type pipe struct {
	ch     chan []byte
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

// Write sends a byte slice to the channel. This function never blocks. If
// the pipe can hold no more data (eg. the reader has stopped reading) then
// PipeFullError may be returned. This function will return len(data) even
// if the reader was not able to read all those bytes.
func (p *pipe) Write(data []byte) (n int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Sending to a closed channel will result in a runtime panic.
	// Instead, if the pipe has been closed, return an error.
	if p.closed {
		return 0, io.ErrClosedPipe
	}
	// "Implementations [of io.Reader] must not retain p."
	data = append([]byte{}, data...)
	select {
	case p.ch <- data:
		return len(data), nil
	default:
		return 0, PipeFullError
	}
}

// Read blocks until data can be read into the provided buffer or until the
// pipe is closed.
func (p *pipe) Read(data []byte) (n int, err error) {
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return 0, io.ErrClosedPipe
	}
	item, ok := <-p.ch
	if !ok {
		return 0, io.ErrClosedPipe
	}
	cnt := len(item)
	if cnt > len(data) {
		cnt = len(data)
	}
	copy(data[:cnt], item[:cnt])
	return cnt, nil
}

// New returns a new pipe that buffers `size` number of writes internally.
// This is conceptually similar to io.Pipe(), except for the differences
// listed in the package description, and the fact that we return only a
// single thing that implements both Reader and Writer.
func New(size int) io.ReadWriteCloser {
	return &pipe{ch: make(chan []byte, size)}
}
