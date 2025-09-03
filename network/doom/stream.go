package doom

import (
	"bytes"
)

const (
	windowSize    = 32
	sendExtraTics = 5
)

// stream represents a particular incoming stream of game packets from one game
// node to another.
type stream struct {
	window           [windowSize][]byte
	windowHead       uint32
	windowTail       uint32
	ipxPacketCounter uint32
}

// handleSetupPacket resets the state of the given stream, and is called
// when an IPXSETUP setup packet is received.
func (s *stream) handleSetupPacket(sp *SetupPacket) {
	// Starting a new game, reset all state.
	s.windowHead = 0
	s.windowTail = 0
	s.ipxPacketCounter = 0
}

func (s *stream) addNewTics(gp *GamePacket) uint32 {
	startTic := ExpandTicNum(gp.StartTic, s.windowHead)

	// Heretic/Hexen games have longer ticcmds than Doom, so don't make
	// any assumptions.
	ticLen := len(gp.Commands) / int(gp.NumTics)

	for i := 0; i < int(gp.NumTics); i++ {
		ticNum := startTic + uint32(i)
		if ticNum < s.windowTail {
			continue
		}
		// We may need to shift the window forward to accomodate the
		// tic we want to add. Once the window is full, we start
		// discarding from the tail end.
		for ticNum >= s.windowHead {
			s.window[s.windowHead%windowSize] = nil
			s.windowHead++
			if s.windowHead-s.windowTail > windowSize {
				s.windowTail = s.windowHead - windowSize
			}
		}
		s.window[ticNum%windowSize] = gp.Commands[ticLen*i : ticLen*(i+1)]
	}

	return startTic + uint32(gp.NumTics)
}

// ticsToSend checks the receive window and calculates a range of tics that
// can be sent in the next game packet.
func (s *stream) ticsToSend(endTic uint32) (uint32, int) {
	count := 0
	idx := endTic
	for count < sendExtraTics && idx > s.windowTail {
		wi := (idx - 1) % windowSize
		if s.window[wi] == nil {
			break
		}
		count++
		idx--
	}
	return idx, count
}

// handleGamePacket processes a Doom game packet and stores tics from it into
// the receive window; it then returns a new packet containing the same tics
// but possibly also additional ones.
func (s *stream) handleGamePacket(gp *GamePacket) *GamePacket {
	// Don't modify game start packets.
	if (gp.Checksum&NcmdSetup) != 0 || gp.NumTics == 0 {
		return gp
	}

	nextTic := s.addNewTics(gp)
	startTic, numTics := s.ticsToSend(nextTic)

	result := *gp
	result.StartTic = byte(startTic & 0xff)
	result.NumTics = byte(numTics & 0xff)

	cmds := make([][]byte, numTics)
	for i := 0; i < numTics; i++ {
		wi := (startTic + uint32(i)) % windowSize
		cmds[i] = s.window[wi]
	}
	result.Commands = bytes.Join(cmds[:], nil)

	return &result
}
