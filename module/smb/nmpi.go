package smb

import (
	"encoding"
	"encoding/binary"
	"fmt"
	"strings"
)

const (
	NmpiOpClaim  = 0xf1
	NmpiOpDelete = 0xf2
	NmpiOpQuery  = 0xf3
	NmpiOpFound  = 0xf4

	NmpiTypeMachine   = 1
	NmpiTypeWorkgroup = 2
	NmpiTypeBrowser   = 3

	nmpiMinFrameLength = 68
)

var (
	_ = (encoding.BinaryMarshaler)(&NmpiFrame{})
	_ = (encoding.BinaryUnmarshaler)(&NmpiFrame{})
)

// NmpiFrame represents an NMPI frame used for NetBIOS name management over
// IPX by the NWLINK implementation.
type NmpiFrame struct {
	routeData     [32]byte
	operation     uint8
	nameType      uint8
	messageID     uint16
	name          string
	service       uint8
	sourceName    string
	sourceService uint8
}

func readNetbiosName(buf []byte) (string, uint8) {
	name := strings.TrimRight(string(buf[:15]), " ")
	return name, buf[15]
}

func (f *NmpiFrame) UnmarshalBinary(data []byte) error {
	if len(data) < nmpiMinFrameLength {
		return fmt.Errorf("NMPI frame too short: %d < %d", len(data), nmpiMinFrameLength)
	}
	copy(f.routeData[:], data[0:32])
	f.operation = data[32]
	f.nameType = data[33]
	f.messageID = binary.LittleEndian.Uint16(data[34:36])
	f.name, f.service = readNetbiosName(data[36:52])
	f.sourceName, f.sourceService = readNetbiosName(data[52:68])
	return nil
}

func putNetbiosName(buf []byte, name string, service uint8) {
	copy(buf, []byte("               "))
	copy(buf, []byte(name))
	buf[15] = service
}

func (f *NmpiFrame) MarshalBinary() ([]byte, error) {
	result := make([]byte, nmpiMinFrameLength)
	copy(result[0:32], f.routeData[:])
	result[32] = f.operation
	result[33] = f.nameType
	binary.LittleEndian.PutUint16(result[34:36], f.messageID)
	putNetbiosName(result[36:52], f.name, f.service)
	putNetbiosName(result[52:68], f.sourceName, f.sourceService)
	return result, nil
}
