package smb

import (
	"fmt"
	"reflect"
	"testing"
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
