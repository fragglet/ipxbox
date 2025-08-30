package doom

import (
	"reflect"
	"testing"
)

var tests = []struct {
	name string
	data []byte
	want *IPXPacket
}{
	{
		name: "Short packet",
		data: []byte{
			0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00,
		},
		want: nil,
	},
	{
		name: "Setup packet decode",
		data: []byte{
			0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00,
			0x01, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00,
		},
		want: &IPXPacket{
			Sequence: 0xffffffff,
			Payload: &SetupPacket{
				NodesFound:  1,
				NodesWanted: 2,
				Extra:       []byte{},
			},
		},
	},
	{
		name: "Short setup packet",
		data: []byte{
			0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00,
			0x01, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00,
		},
		want: nil,
	},
	{
		name: "Game packet decode",
		data: []byte{
			0x01, 0x00, 0x00, 0x00, 0x67, 0x49, 0x24, 0x02,
			0x00, 0x04, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		},
		want: &IPXPacket{
			Sequence: 1,
			Payload: &GamePacket{
				Checksum: 0x2244967,
				StartTic: 4,
				Player:   1,
				NumTics:  1,
				Commands: []byte{
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				},
			},
		},
	},
	{
		name: "Game start packet decode",
		data: []byte{
			0x01, 0x00, 0x00, 0x00, 0x69, 0x86, 0x90, 0x21,
			0x02, 0x41, 0x6d, 0x00, 0x00, 0x00, 0x00, 0x00,
		},
		want: &IPXPacket{
			Sequence: 1,
			Payload: &GamePacket{
				Checksum:       0x21908669,
				RetransmitFrom: 2,
				StartTic:       65,
				Player:         109,
				NumTics:        0,
				Commands:       []byte{},
			},
		},
	},
	{
		name: "Short game packet",
		data: []byte{
			0x01, 0x00, 0x00, 0x00, 0x67, 0x49, 0x24, 0x02,
			0x00, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00,
		},
		want: nil,
	},
}

func TestPacketUnmarshal(t *testing.T) {
	for _, test := range tests {

		t.Run(test.name, func(t *testing.T) {
			var got IPXPacket
			err := got.UnmarshalBinary(test.data)
			switch {
			case test.want != nil && err != nil:
				t.Errorf("unmarshal failed, got error %v", err)
			case test.want == nil && err == nil:
				t.Errorf("want error, got no error")
			case test.want != nil && !reflect.DeepEqual(&got, test.want):
				t.Errorf("unmarshal incorrect, want: %+v\ngot: %+v\nwant payload: %+v\ngot payload: %+v", test.want, &got, test.want.Payload, got.Payload)
			}
		})
	}
}

func TestPacketMarshal(t *testing.T) {
	for _, test := range tests {
		if test.want == nil {
			continue
		}

		t.Run(test.name, func(t *testing.T) {
			data, err := test.want.MarshalBinary()
			switch {
			case err != nil:
				t.Errorf("marshal failed, got error %v", err)
			case !reflect.DeepEqual(data, test.data):
				t.Errorf("marshal incorrect, want: %+v\ngot: %+v", test.data, data)
			}
		})
	}
}
