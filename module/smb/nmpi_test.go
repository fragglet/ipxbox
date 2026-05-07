package smb

import (
	"context"
	"reflect"
	"testing"

	"github.com/fragglet/ipxbox/ipx"
	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/network/addressable"
	"github.com/fragglet/ipxbox/network/ipxswitch"
)

func TestFrameMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    NmpiFrame
		wantErr bool
	}{
		{
			name: "Query packet",
			data: []byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0xf3, 0x01, 0x01, 0x80, 0x42, 0x4c, 0x45, 0x48,
				0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20,
				0x20, 0x20, 0x20, 0x20, 0x50, 0x4f, 0x52, 0x54,
				0x59, 0x39, 0x38, 0x20, 0x20, 0x20, 0x20, 0x20,
				0x20, 0x20, 0x20, 0x00,
			},
			want: NmpiFrame{
				operation:     NmpiOpQuery,
				nameType:      NmpiTypeMachine,
				messageID:     0x8001,
				name:          "BLEH",
				service:       0x20,
				sourceName:    "PORTY98",
				sourceService: 0,
			},
		},
		{
			name: "Truncated packet",
			data: []byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got NmpiFrame
			err := got.UnmarshalBinary(test.data)
			switch {
			case test.wantErr && err == nil:
				t.Errorf("want error, got none")
			case !test.wantErr && err != nil:
				t.Errorf("unexpected error %v", err)
			case !reflect.DeepEqual(&got, &test.want):
				t.Errorf("unmarshaled wrong: want %#v got %#v", &test.want, &got)
			}

			// Don't bother remarshaling for error case:
			if test.wantErr {
				return
			}

			gotData, err := got.MarshalBinary()
			switch {
			case err != nil:
				t.Errorf("error during remarshal: %v", err)
			case !reflect.DeepEqual(test.data, gotData):
				t.Errorf("did not remarshal correctly:\nwant %+v\ngot %+v",
					test.data, gotData)
			}
		})
	}
}

func TestAnnouncer(t *testing.T) {
	net := addressable.Wrap(ipxswitch.New())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// First announcer should register successfully.
	{
		node, err := net.NewNode()
		if err != nil {
			t.Errorf("error creating node: %v", err)
		}
		testAddr := ipx.HeaderAddr{
			Addr:   network.NodeAddress(node),
			Socket: 0x553,
		}
		a := NewNmpiAnnouncer("FOOBAR", 0, testAddr, node)
		go ipx.CopyPackets(ctx, node, a)
		if err = a.Register(); err != nil {
			t.Errorf("failed to register primary announcer: %v", err)
		}
	}

	// If we send a query to the network, we should get a reply.
	{
		node, err := net.NewNode()
		if err != nil {
			t.Errorf("error creating node: %v", err)
		}
		queryAddr := ipx.HeaderAddr{
			Addr:   network.NodeAddress(node),
			Socket: 0x553,
		}
		msg := &NmpiFrame{
			operation:     NmpiOpQuery,
			nameType:      NmpiTypeMachine,
			name:          "FOOBAR",
			service:       0,
			sourceName:    "BAZ",
			sourceService: 0,
		}
		payload, _ := msg.MarshalBinary()
		err = node.WritePacket(&ipx.Packet{
			Header: ipx.Header{
				Dest: ipx.HeaderAddr{
					Addr:   ipx.AddrBroadcast,
					Socket: nmpiSocket,
				},
				Src: queryAddr,
			},
			Payload: payload,
		})
		if err != nil {
			t.Errorf("error writing packet: %v", err)
		}
		// Read the reply the announcer sends back:
		pkt, err := node.ReadPacket(ctx)
		if err != nil {
			t.Errorf("error reading packet: %v", err)
		}
		if pkt.Header.Dest != queryAddr {
			t.Errorf("reply to wrong address: want %v, got %v", queryAddr, pkt.Header.Dest)
		}
		var got NmpiFrame
		err = got.UnmarshalBinary(pkt.Payload)
		if err != nil {
			t.Errorf("error unmarshaling payload: %v", err)
		}
		want := &NmpiFrame{
			operation:     NmpiOpFound,
			nameType:      NmpiTypeMachine,
			name:          "BAZ",
			service:       0,
			sourceName:    "FOOBAR",
			sourceService: 0,
		}
		if !reflect.DeepEqual(want, &got) {
			t.Errorf("wrong query response:\nwant %#v\ngot %#v", want, &got)
		}
	}

	// We add a second announcer to the network; when it tries to
	// register it should fail.
	{
		node, err := net.NewNode()
		if err != nil {
			t.Errorf("error creating node: %v", err)
		}
		testAddr := ipx.HeaderAddr{
			Addr:   network.NodeAddress(node),
			Socket: 0x553,
		}
		a := NewNmpiAnnouncer("FOOBAR", 0, testAddr, node)
		go ipx.CopyPackets(ctx, node, a)
		if err = a.Register(); err == nil {
			t.Errorf("expected secondary announcer to fail, got success")
		}
	}
}
