package pipe

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/fragglet/ipxbox/ipx"
)

var (
	testPacket = &ipx.Packet{
		Header: ipx.Header{
			Checksum:     0xffff,
			Length:       9999,
			TransControl: 12,
			PacketType:   34,
			Dest: ipx.HeaderAddr{
				Network: [4]byte{1, 2, 3, 4},
				Addr:    ipx.AddrBroadcast,
				Socket:  0x4567,
			},
			Src: ipx.HeaderAddr{
				Network: [4]byte{0x43, 0x21, 0x87, 0x65},
				Addr:    [6]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66},
				Socket:  0x4567,
			},
		},
		Payload: []byte("hello"),
	}
)

func makeTestPackets(count int) []*ipx.Packet {
	result := []*ipx.Packet{}
	for i := 0; i < count; i++ {
		pkt := &ipx.Packet{}
		*pkt = *testPacket
		pkt.Payload = []byte(fmt.Sprintf("packet number %d", i+1))
		result = append(result, pkt)
	}
	return result
}

func TestWriteThenRead(t *testing.T) {
	p := New()
	wantPackets := makeTestPackets(10)
	for _, pkt := range wantPackets {
		if err := p.WritePacket(pkt); err != nil {
			t.Errorf("failed WritePacket: %v", err)
			return
		}
	}
	gotPackets := []*ipx.Packet{}
	ctx := context.Background()
	for i := 0; i < len(wantPackets); i++ {
		pkt, err := p.ReadPacket(ctx)
		if err != nil {
			t.Errorf("failed ReadPacket: %v", err)
			return
		}
		gotPackets = append(gotPackets, pkt)
	}
	if !reflect.DeepEqual(gotPackets, wantPackets) {
		t.Errorf("packets read back wrong: want:%+v, got %+v", wantPackets, gotPackets)
	}
}

func TestNeverBlocks(t *testing.T) {
	p := New()
	for i := 0; i < 1000; i++ {
		err := p.WritePacket(testPacket)
		if err != nil && err != PipeFullError {
			t.Errorf("error writing packet: %v", err)
			return
		}
	}
}

func TestExpiredContext(t *testing.T) {
	p := New()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	_, err := p.ReadPacket(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("want error %v, got %v", context.DeadlineExceeded, err)
	}
	cancel()
}

func TestClosingSocket(t *testing.T) {
	ctx := context.Background()
	p := New()
	go func() {
		time.Sleep(1 * time.Second)
		p.Close()
	}()

	_, err := p.ReadPacket(ctx)
	if err != io.ErrClosedPipe {
		t.Errorf("want error %v, got %v", io.ErrClosedPipe, err)
	}
}
