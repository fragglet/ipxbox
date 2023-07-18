package testing

import (
	"context"
	"log"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/network/pipe"
)

type fakeAddr struct{}

func (*fakeAddr) Network() string { return "fake" }
func (*fakeAddr) String() string  { return "testing" }

var FakeAddress = &fakeAddr{}

var TestPackets = []*ipx.Packet{
	{
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
	},
	{
		Header: ipx.Header{
			Checksum:     0x1234,
			Length:       9123,
			TransControl: 123,
			PacketType:   251,
			Dest: ipx.HeaderAddr{
				Network: [4]byte{0x99, 0x11, 0x18, 0x1b},
				Addr:    ipx.AddrNull,
				Socket:  0x942f,
			},
			Src: ipx.HeaderAddr{
				Network: [4]byte{0x8d, 0x5b, 0x11, 0x50},
				Addr:    [6]byte{0x29, 0x5f, 0x21, 0x8d, 0x84, 0x91},
				Socket:  0x8d2b,
			},
		},
		Payload: []byte("another packet!"),
	},
	{
		Header: ipx.Header{
			Checksum:     0x8d2a,
			Length:       33,
			TransControl: 0x92,
			PacketType:   0x1b,
			Dest: ipx.HeaderAddr{
				Network: [4]byte{0x8d, 0x9b, 0x1c, 0xc4},
				Addr:    [6]byte{0xb1, 0x9c, 0x1a, 0xd5, 0xb1, 0x9f},
				Socket:  0x83b1,
			},
			Src: ipx.HeaderAddr{
				Network: [4]byte{0x8d, 0x1b, 0x66, 0x82},
				Addr:    [6]byte{0x83, 0xb1, 0x3d, 0x78, 0x81, 0xbb},
				Socket:  0x21b6,
			},
		},
		Payload: []byte("packet number 3"),
	},
	{
		Header: ipx.Header{
			Checksum:     0x9ab1,
			Length:       941,
			TransControl: 0x8a,
			PacketType:   0x92,
			Dest: ipx.HeaderAddr{
				Addr:   [6]byte{0x02, 0x82, 0xb1, 0x22, 0x1a, 0xa8},
				Socket: 0x82c1,
			},
			Src: ipx.HeaderAddr{
				Network: [4]byte{0x9a, 0x93, 0xb2, 0xaa},
				Addr:    [6]byte{0x9c, 0xcc, 0x99, 0xa8, 0x87, 0xb1},
				Socket:  0x9b88,
			},
		},
		Payload: []byte("the last test packet of the bunch."),
	},
}

type LoopbackEnd struct {
	side   string
	other  *LoopbackEnd
	rxpipe ipx.ReadWriteCloser
}

func (e *LoopbackEnd) ReadPacket(ctx context.Context) (*ipx.Packet, error) {
	result, err := e.rxpipe.ReadPacket(ctx)
	if err != nil {
		log.Printf("%v: ReadPacket returned error: %v", e.side, err)
	} else {
		log.Printf("%v: ReadPacket returned packet: %+v", e.side, result)
	}
	return result, err
}

func (e *LoopbackEnd) WritePacket(pkt *ipx.Packet) error {
	log.Printf("%v: WritePacket: %+v", e.side, pkt)
	return e.other.rxpipe.WritePacket(pkt)
}

func (e *LoopbackEnd) Close() error {
	e.rxpipe.Close()
	return nil
}

func MakeLoopbackPair(side1, side2 string) (*LoopbackEnd, *LoopbackEnd) {
	x := &LoopbackEnd{
		side:   side1,
		rxpipe: pipe.New(),
	}
	y := &LoopbackEnd{
		side:   side2,
		rxpipe: pipe.New(),
	}
	x.other = y
	y.other = x
	return x, y
}

// CallbackDest invokes a callback function when a packet is sent
// to it, and has a SendPacket method to send replies to code under test.
type CallbackDest struct {
	callback func(pkt *ipx.Packet)
	rxpipe   ipx.ReadWriteCloser
}

func (d *CallbackDest) ReadPacket(ctx context.Context) (*ipx.Packet, error) {
	return d.rxpipe.ReadPacket(ctx)
}

func (d *CallbackDest) WritePacket(pkt *ipx.Packet) error {
	d.callback(pkt)
	return nil
}

func (d *CallbackDest) SendPacket(pkt *ipx.Packet) error {
	return d.rxpipe.WritePacket(pkt)
}

func (d *CallbackDest) Close() error {
	return d.rxpipe.Close()
}

func MakeCallbackDest(callback func(pkt *ipx.Packet)) *CallbackDest {
	return &CallbackDest{
		callback: callback,
		rxpipe:   pipe.New(),
	}
}

// FakeNetwork is an implementation of network.Network and network.Node
// for testing that returns itself when NewNode() is called.
type FakeNetwork struct {
	Inner   ipx.ReadWriteCloser
	Address ipx.Addr
}

func (n *FakeNetwork) NewNode() network.Node {
	return n
}

func (n *FakeNetwork) ReadPacket(ctx context.Context) (*ipx.Packet, error) {
	if n.Inner == nil {
		_ = <-ctx.Done()
		return nil, ctx.Err()
	}
	return n.Inner.ReadPacket(ctx)
}

func (n *FakeNetwork) WritePacket(pkt *ipx.Packet) error {
	if n.Inner != nil {
		return n.Inner.WritePacket(pkt)
	}
	return nil
}

func (n *FakeNetwork) Close() error {
	if n.Inner != nil {
		n.Inner.Close()
	}
	return nil
}

func (n *FakeNetwork) GetProperty(value interface{}) bool {
	switch value.(type) {
	case *ipx.Addr:
		*value.(*ipx.Addr) = n.Address
		return true
	default:
		return false
	}
}
