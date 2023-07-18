package filter

import (
	"testing"

	"github.com/fragglet/ipxbox/ipx"
	ipxtesting "github.com/fragglet/ipxbox/testing"
)

const (
	goodSocket = 9999
	badSocket  = 0x455 // NetBIOS
)

func makeTestPacket(src, dest uint16) *ipx.Packet {
	return &ipx.Packet{
		Header: ipx.Header{
			Src: ipx.HeaderAddr{
				Addr:   ipx.AddrNull,
				Socket: src,
			},
			Dest: ipx.HeaderAddr{
				Addr:   ipx.AddrBroadcast,
				Socket: dest,
			},
		},
	}
}

func TestFilteredWrites(t *testing.T) {
	gotPackets := 0
	var lastPacket *ipx.Packet
	dest := ipxtesting.MakeCallbackDest(func(pkt *ipx.Packet) {
		gotPackets++
		lastPacket = pkt
	})
	defer dest.Close()

	filter := New(dest)

	t.Run("bad dest socket", func(t *testing.T) {
		testPacket := makeTestPacket(goodSocket, badSocket)
		err := filter.WritePacket(testPacket)
		if err != FilteredPacketError {
			t.Errorf("want error %v, got %v", FilteredPacketError, err)
		}
		if gotPackets != 0 {
			t.Errorf("packet passed through filter: gotPackets=%d, lastPacket=%+v", gotPackets, lastPacket)
		}
	})
	t.Run("bad src socket", func(t *testing.T) {
		testPacket := makeTestPacket(badSocket, goodSocket)
		err := filter.WritePacket(testPacket)
		if err != FilteredPacketError {
			t.Errorf("want error %v, got %v", FilteredPacketError, err)
		}
		if gotPackets != 0 {
			t.Errorf("packet passed through filter: gotPackets=%d, lastPacket=%+v", gotPackets, lastPacket)
		}
	})
	t.Run("unfiltered packet", func(t *testing.T) {
		testPacket := makeTestPacket(goodSocket, goodSocket)
		err := filter.WritePacket(testPacket)
		if err != nil {
			t.Errorf("error on WritePacket: %v", err)
		}
		if gotPackets != 1 {
			t.Errorf("want gotPackets=1, got=%d", gotPackets)
		} else if testPacket != lastPacket {
			t.Errorf("wrong packet passed through filter: want %+v, got %+v", testPacket, lastPacket)
		}
	})
}
