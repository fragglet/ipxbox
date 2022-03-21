package ipx

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"testing"
	"time"
)

var (
	testPackets = []*Packet{
		{
			Header: Header{
				Checksum:     0xffff,
				Length:       9999,
				TransControl: 12,
				PacketType:   34,
				Dest: HeaderAddr{
					Network: [4]byte{1, 2, 3, 4},
					Addr:    AddrBroadcast,
					Socket:  0x4567,
				},
				Src: HeaderAddr{
					Network: [4]byte{0x43, 0x21, 0x87, 0x65},
					Addr:    [6]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66},
					Socket:  0x4567,
				},
			},
			Payload: []byte("hello"),
		},
		{
			Header: Header{
				Checksum:     0x1234,
				Length:       9123,
				TransControl: 123,
				PacketType:   251,
				Dest: HeaderAddr{
					Network: [4]byte{0x99, 0x11, 0x18, 0x1b},
					Addr:    AddrNull,
					Socket:  0x942f,
				},
				Src: HeaderAddr{
					Network: [4]byte{0x8d, 0x5b, 0x11, 0x50},
					Addr:    [6]byte{0x29, 0x5f, 0x21, 0x8d, 0x84, 0x91},
					Socket:  0x8d2b,
				},
			},
			Payload: []byte("another packet!"),
		},
		{
			Header: Header{
				Checksum:     0x8d2a,
				Length:       33,
				TransControl: 0x92,
				PacketType:   0x1b,
				Dest: HeaderAddr{
					Network: [4]byte{0x8d, 0x9b, 0x1c, 0xc4},
					Addr:    [6]byte{0xb1, 0x9c, 0x1a, 0xd5, 0xb1, 0x9f},
					Socket:  0x83b1,
				},
				Src: HeaderAddr{
					Network: [4]byte{0x8d, 0x1b, 0x66, 0x82},
					Addr:    [6]byte{0x83, 0xb1, 0x3d, 0x78, 0x81, 0xbb},
					Socket:  0x21b6,
				},
			},
			Payload: []byte("packet number 3"),
		},
		{
			Header: Header{
				Checksum:     0x9ab1,
				Length:       941,
				TransControl: 0x8a,
				PacketType:   0x92,
				Dest: HeaderAddr{
					Addr:   [6]byte{0x02, 0x82, 0xb1, 0x22, 0x1a, 0xa8},
					Socket: 0x82c1,
				},
				Src: HeaderAddr{
					Network: [4]byte{0x9a, 0x93, 0xb2, 0xaa},
					Addr:    [6]byte{0x9c, 0xcc, 0x99, 0xa8, 0x87, 0xb1},
					Socket:  0x9b88,
				},
			},
			Payload: []byte("the last test packet of the bunch."),
		},
	}
)

type TestingReadWriteCloser struct {
	sourcePackets []*Packet
	sourceIndex   int
	recvPackets   []*Packet
	finalError    error
}

func (r *TestingReadWriteCloser) ReadPacket(ctx context.Context) (*Packet, error) {
	if r.sourceIndex >= len(r.sourcePackets) {
		if r.finalError != nil {
			return nil, r.finalError
		} else {
			_ = <-ctx.Done()
			return nil, ctx.Err()
		}
	}
	result := &Packet{}
	*result = *r.sourcePackets[r.sourceIndex]
	r.sourceIndex++
	return result, nil
}

func (r *TestingReadWriteCloser) WritePacket(packet *Packet) error {
	r.recvPackets = append(r.recvPackets, packet)
	return nil
}

func (r *TestingReadWriteCloser) Close() error {
	return nil
}

func TestMarshal(t *testing.T) {
	pkt := testPackets[0]
	gotBytes, err := pkt.MarshalBinary()
	if err != nil {
		t.Fatalf("pkt.Marshal failed: %v", err)
	}
	wantBytes := []byte{
		0xff, 0xff, 0x27, 0x0f, 0x0c, 0x22, 0x01, 0x02,
		0x03, 0x04, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0x45, 0x67, 0x43, 0x21, 0x87, 0x65, 0x11, 0x22,
		0x33, 0x44, 0x55, 0x66, 0x45, 0x67, 0x68, 0x65,
		0x6c, 0x6c, 0x6f,
	}
	if !bytes.Equal(gotBytes, wantBytes) {
		t.Errorf("pkt.Marshal wrong: want %+v, got %+v", wantBytes, gotBytes)
	}
	var pkt2 Packet
	if err := pkt2.UnmarshalBinary(wantBytes); err != nil {
		t.Fatalf("pkt2.Unmarshal failed: %v", err)
	}
	if !reflect.DeepEqual(pkt, &pkt2) {
		t.Errorf("pkt2.Unmarshal wrong: want %+v, got %+v", &pkt, &pkt2)
	}
}

func TestShortPacket(t *testing.T) {
	pktBytes := []byte{0x01, 0x02, 0x03, 0x04}
	var pkt Packet
	if err := pkt.UnmarshalBinary(pktBytes); err == nil {
		t.Errorf("want error, got none")
	}
}

func TestCopyPackets(t *testing.T) {
	t.Run("Copy until EOF", func(t *testing.T) {
		var x, y TestingReadWriteCloser
		x.finalError = io.EOF
		x.sourcePackets = testPackets[0:2]
		y.finalError = io.EOF
		y.sourcePackets = testPackets[2:]
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(5 * time.Second)
			cancel()
		}()
		gotErr := DuplexCopyPackets(ctx, &x, &y)
		if gotErr != nil {
			t.Errorf("got error: %v", gotErr)
		}
		if !reflect.DeepEqual(x.sourcePackets, y.recvPackets) {
			t.Errorf("not all packets copied from x to y: want %+v, got %+v", x.sourcePackets, y.recvPackets)
		}
		if !reflect.DeepEqual(x.sourcePackets, y.recvPackets) {
			t.Errorf("not all packets copied from y to x: want %+v, got %+v", y.sourcePackets, x.recvPackets)
		}
	})

	t.Run("Closed pipe", func(t *testing.T) {
		var x, y TestingReadWriteCloser
		x.finalError = io.ErrClosedPipe
		x.sourcePackets = testPackets[0:2]
		y.sourcePackets = testPackets[2:]
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(5 * time.Second)
			cancel()
		}()
		gotErr := DuplexCopyPackets(ctx, &x, &y)
		if gotErr != io.ErrClosedPipe {
			t.Errorf("wrong error: want %v, got %v", io.ErrClosedPipe, gotErr)
		}
		if !reflect.DeepEqual(x.sourcePackets, y.recvPackets) {
			t.Errorf("not all packets copied from x to y: want %+v, got %+v", x.sourcePackets, y.recvPackets)
		}
	})

	t.Run("Context canceled", func(t *testing.T) {
		var x, y TestingReadWriteCloser
		x.sourcePackets = testPackets[0:2]
		y.sourcePackets = testPackets[2:]
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(5 * time.Second)
			cancel()
		}()
		gotErr := DuplexCopyPackets(ctx, &x, &y)
		if gotErr != context.Canceled {
			t.Errorf("wrong error: want %v, got %v", io.ErrClosedPipe, gotErr)
		}
		if !reflect.DeepEqual(x.sourcePackets, y.recvPackets) {
			t.Errorf("not all packets copied from x to y: want %+v, got %+v", x.sourcePackets, y.recvPackets)
		}
		if !reflect.DeepEqual(x.sourcePackets, y.recvPackets) {
			t.Errorf("not all packets copied from y to x: want %+v, got %+v", y.sourcePackets, x.recvPackets)
		}
	})
}
