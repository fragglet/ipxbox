package smb

import (
	"encoding"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fragglet/ipxbox/ipx"
)

const (
	NmpiOpClaim  = 0xf1
	NmpiOpDelete = 0xf2
	NmpiOpQuery  = 0xf3
	NmpiOpFound  = 0xf4

	NmpiTypeMachine   = 1
	NmpiTypeWorkgroup = 2
	NmpiTypeBrowser   = 3

	nmpiSocket         = 0x551
	nmpiMinFrameLength = 68

	// From smbpub.txt:
	// When the server starts, it sends broadcasts a name claim
	// (inm_type == INAME_CLAIM) packet five (5) times at 500
	// millisecond intervals.
	registrationAttempts = 5
	registrationSpacing  = 500 * time.Millisecond
)

var (
	_ = (encoding.BinaryMarshaler)(&NmpiFrame{})
	_ = (encoding.BinaryUnmarshaler)(&NmpiFrame{})

	ErrRegistrationConflict = errors.New("name conflict during registration")
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

type announcerState int

const (
	stateRegistering = announcerState(iota)
	stateConflict
	stateRegistered
)

// NmpiAnnouncer registers a NetBIOS name on the network and responds
// to queries for that name.
type NmpiAnnouncer struct {
	state        announcerState
	name         string
	service      uint8
	addr         ipx.HeaderAddr
	writer       ipx.Writer
	nameConflict bool
}

func NewNmpiAnnouncer(name string, service uint8, addr ipx.HeaderAddr, writer ipx.Writer) *NmpiAnnouncer {
	return &NmpiAnnouncer{
		name:    name,
		service: service,
		addr:    addr,
		writer:  writer,
	}
}

// handleQuery is called when an NMPI "query" or "claim" frame is received.
func (a *NmpiAnnouncer) handleQuery(pkt *ipx.Packet, f *NmpiFrame) error {

	// Only reply after registration has succeeded.
	if a.state != stateRegistered {
		return nil
	}

	reply := *f
	reply.operation = NmpiOpFound

	payload, _ := reply.MarshalBinary()

	return a.writer.WritePacket(&ipx.Packet{
		Header: ipx.Header{
			Dest: pkt.Header.Src,
			Src:  a.addr,
		},
		Payload: payload,
	})
}

// handleClaim is called when an NMPI "found" frame is received.
func (a *NmpiAnnouncer) handleFound(pkt *ipx.Packet, f *NmpiFrame) error {
	if a.state == stateRegistering {
		a.state = stateConflict
	}

	return nil
}

// WritePacket processes a packet received at the NMPI socket number.
func (a *NmpiAnnouncer) WritePacket(pkt *ipx.Packet) error {
	var f NmpiFrame
	if err := f.UnmarshalBinary(pkt.Payload); err != nil {
		return err
	}
	// We only care about our name.
	if f.name != a.name || f.service != a.service {
		return nil
	}
	// Loopback packet?
	if pkt.Header.Src == a.addr {
		return nil
	}
	switch f.operation {
	case NmpiOpClaim, NmpiOpQuery:
		return a.handleQuery(pkt, &f)
	case NmpiOpFound:
		return a.handleFound(pkt, &f)
	}
	return nil
}

// Register sends several NMPI registration packets to attempt to the network
// to see if anyone else is using the same name. It blocks until this process
// completes. If successful, the announcer will start responding to name
// queries.
func (a *NmpiAnnouncer) Register() error {
	a.state = stateRegistering

	msg := &NmpiFrame{
		operation:     NmpiOpClaim,
		nameType:      NmpiTypeMachine,
		name:          a.name,
		service:       a.service,
		sourceName:    a.name,
		sourceService: a.service,
	}
	payload, _ := msg.MarshalBinary()

	for i := 0; i < registrationAttempts; i++ {
		err := a.writer.WritePacket(&ipx.Packet{
			Header: ipx.Header{
				Dest: ipx.HeaderAddr{
					Addr:   ipx.AddrBroadcast,
					Socket: nmpiSocket,
				},
				Src: a.addr,
			},
			Payload: payload,
		})
		if err != nil {
			return err
		}

		// Wait 500ms before sending another registration attempt.
		// After the pause, we check to see if there was a conflict.
		time.Sleep(registrationSpacing)
		if a.state == stateConflict {
			return ErrRegistrationConflict
		}
	}

	// Registration successful
	a.state = stateRegistered

	return nil
}
