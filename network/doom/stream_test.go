package doom

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"reflect"
	"testing"
)

func TestStream(t *testing.T) {
	packets := []struct {
		input *GamePacket
		want  *GamePacket
	}{
		{ // #0
			input: &GamePacket{
				StartTic: 0,
				NumTics:  1,
				Commands: []byte{0},
			},
			want: &GamePacket{
				StartTic: 0,
				NumTics:  1,
				Commands: []byte{0},
			},
		},

		{ // #1
			input: &GamePacket{
				StartTic: 2,
				NumTics:  3,
				Commands: []byte{2, 3, 4},
			},
			// Out of order receive means we only get the tics
			// from this packet.
			want: &GamePacket{
				StartTic: 2,
				NumTics:  3,
				Commands: []byte{2, 3, 4},
			},
		},

		{ // #2
			input: &GamePacket{
				StartTic: 1,
				NumTics:  1,
				Commands: []byte{1},
			},
			want: &GamePacket{
				StartTic: 0,
				NumTics:  2,
				Commands: []byte{0, 1},
			},
		},

		{ // #3
			input: &GamePacket{
				StartTic: 5,
				NumTics:  2,
				Commands: []byte{5, 6},
			},
			// At this point we have more than five tics in the window,
			// but we only send the most recent five.
			want: &GamePacket{
				StartTic: 2,
				NumTics:  5,
				Commands: []byte{2, 3, 4, 5, 6},
			},
		},
	}

	var s stream
	s.handleSetupPacket(nil)

	for idx, packet := range packets {
		got := s.handleGamePacket(packet.input)
		if !reflect.DeepEqual(got, packet.want) {
			t.Errorf("packet #%d: wrong output:\nwant %+v\ngot %+v",
				idx, packet.want, got)
		}
	}
}

func TestStreamReset(t *testing.T) {
	var s stream
	s.handleSetupPacket(nil)

	_ = s.handleGamePacket(&GamePacket{
		StartTic: 0,
		NumTics:  1,
		Commands: []byte{0},
	})

	// Setup packet indicates start of new game:
	s.handleSetupPacket(nil)

	// If we deliver another tic now, the output should not include
	// the previously stored tic.
	got := s.handleGamePacket(&GamePacket{
		StartTic: 1,
		NumTics:  1,
		Commands: []byte{1},
	})
	want := &GamePacket{
		StartTic: 1,
		NumTics:  1,
		Commands: []byte{1},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrong output:\nwant %+v\ngot %+v", want, got)
	}
}

func ticData(start, count int) []byte {
	hashes := [][]byte{}
	for i := 0; i < count; i++ {
		hash := sha256.Sum256([]byte(fmt.Sprintf("%d", start+i)))
		hashes = append(hashes, hash[:])
	}
	return bytes.Join(hashes[:], nil)
}

// TestLongStream simulates a long-running stream where we send many ticcmds
// and wrap the window many times.
func TestLongStream(t *testing.T) {
	var s stream
	s.handleSetupPacket(nil)

	for i := 0; i < 100000; i++ {
		got := s.handleGamePacket(&GamePacket{
			StartTic: byte(i & 0xff),
			NumTics:  1,
			Commands: ticData(i, 1),
		})
		if got.NumTics < 1 {
			t.Errorf("packet #%d: want at least one tic, got none", i)
		}
		startTic := i + 1 - int(got.NumTics)
		want := &GamePacket{
			StartTic: byte(startTic & 0xff),
			NumTics:  got.NumTics,
			Commands: ticData(startTic, int(got.NumTics)),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("packet #%d: wrong output:\nwant %+v\ngot %+v",
				i, want, got)
		}
	}
}
