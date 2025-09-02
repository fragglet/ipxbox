package doom

import (
	"encoding"
	"encoding/binary"
	"errors"
)

const (
	NcmdExit       = 0x80000000
	NcmdRetransmit = 0x40000000
	NcmdSetup      = 0x20000000
	NcmdKill       = 0x10000000
	NcmdChecksum   = 0x0fffffff
)

var (
	ErrShortPacket = errors.New("packet too short")
)

// GamePacket contains a decoded form of the data found in Doom's network
// protocol messages. This layer is used in all Doom games and is not specific
// to just IPX games.
type GamePacket struct {
	Checksum       uint32
	RetransmitFrom byte
	StartTic       byte
	Player         byte
	NumTics        byte
	Commands       []byte
}

// ExpandTicNum takes the low byte of a tic number and "expands" it back
// to a 32-bit value, using the base parameter as a reference.
func ExpandTicNum(low byte, base uint32) uint32 {
	result := (base & ^uint32(0xff)) | uint32(low)
	switch {
	case result > base && result-base > 64:
		result -= 256
	case result < base && base-result > 64:
		result += 256
	}
	return result
}

// GamePacketChecksum calculates the checksum for a marshaled GamePacket;
// it should be called with the first four bytes stripped.
func GamePacketChecksum(data []byte) uint32 {
	result := uint32(0x1234567)
	offset := 0
	numDwords := len(data) / 4
	for i := 0; i < numDwords; i++ {
		result += binary.LittleEndian.Uint32(data[offset:offset+4]) * uint32(i + 1)
		offset += 4
	}
	return result & NcmdChecksum
}

// UpdateChecksum updates the Checksum field of the given GamePacket. The
// high bits that store additional information (setup, retransmit, etc.) are
// left untouched.
func (p *GamePacket) UpdateChecksum() {
	marshaled, err := p.MarshalBinary()
	if err != nil {
		panic(err)
	}
	checksum := GamePacketChecksum(marshaled[4:])
	p.Checksum = (p.Checksum & ^uint32(NcmdChecksum)) | checksum
}

func (p *GamePacket) MarshalBinary() ([]byte, error) {
	var hdr [8]byte

	binary.LittleEndian.PutUint32(hdr[0:4], p.Checksum)
	hdr[4] = p.RetransmitFrom
	hdr[5] = p.StartTic
	hdr[6] = p.Player
	hdr[7] = p.NumTics

	return append(hdr[:], p.Commands...), nil
}

func (p *GamePacket) UnmarshalBinary(data []byte) error {
	if len(data) < 8 {
		return ErrShortPacket
	}
	p.Checksum = binary.LittleEndian.Uint32(data[0:4])
	p.RetransmitFrom = data[4]
	p.StartTic = data[5]
	p.Player = data[6]
	p.NumTics = data[7]
	p.Commands = append([]byte{}, data[8:]...)
	return nil
}

// SetupPacket contains a decoded form of the data found in Doom IPX
// setup packets. These are messages generated and interpreted by the
// ipxsetup.exe network driver.
type SetupPacket struct {
	GameID      uint16
	Drone       uint16
	NodesFound  uint16
	NodesWanted uint16
	Extra       []byte
}

func (p *SetupPacket) MarshalBinary() ([]byte, error) {
	var result [8]byte
	binary.LittleEndian.PutUint16(result[0:2], p.GameID)
	binary.LittleEndian.PutUint16(result[2:4], p.Drone)
	binary.LittleEndian.PutUint16(result[4:6], p.NodesFound)
	binary.LittleEndian.PutUint16(result[6:8], p.NodesWanted)
	return append(result[:], p.Extra...), nil
}

func (p *SetupPacket) UnmarshalBinary(data []byte) error {
	if len(data) < 8 {
		return ErrShortPacket
	}
	p.GameID = binary.LittleEndian.Uint16(data[0:2])
	p.Drone = binary.LittleEndian.Uint16(data[2:4])
	p.NodesFound = binary.LittleEndian.Uint16(data[4:6])
	p.NodesWanted = binary.LittleEndian.Uint16(data[6:8])
	p.Extra = append([]byte{}, data[8:]...)
	return nil
}

type Payload interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

// IPXPacket contains a decoded form of the wrapper format used for Doom
// IPX packets.
type IPXPacket struct {
	Sequence, Tail uint32
	Payload        Payload
}

func (p *IPXPacket) MarshalBinary() ([]byte, error) {
	var header, tail [4]byte
	binary.LittleEndian.PutUint32(header[:], p.Sequence)
	binary.LittleEndian.PutUint32(tail[:], p.Tail)
	payload, err := p.Payload.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return append(append(header[:], payload...), tail[:]...), nil
}

func (p *IPXPacket) UnmarshalBinary(data []byte) error {
	if len(data) < 8 {
		return ErrShortPacket
	}
	p.Sequence = binary.LittleEndian.Uint32(data[0:4])
	p.Tail = binary.LittleEndian.Uint32(data[len(data)-4:])

	var payload Payload

	if p.Sequence == 0xffffffff {
		payload = &SetupPacket{}
	} else {
		payload = &GamePacket{}
	}

	err := payload.UnmarshalBinary(data[4 : len(data)-4])
	if err != nil {
		return err
	}
	p.Payload = payload
	return nil
}
