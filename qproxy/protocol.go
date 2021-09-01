package qproxy

import (
	"encoding/binary"
	"errors"
)

const (
	reliableHeaderLength = 8
	vanillaQuakeMTU      = 1024

	flagData       = uint16(0x0001)
	flagAck        = uint16(0x0002)
	flagNak        = uint16(0x0004)
	flagEOM        = uint16(0x0008)
	flagUnreliable = uint16(0x0010)
	flagCtl        = uint16(0x8000)
)

type state uint8

const (
	// Receiving message fragments from upstream and transmitting
	// to downstream. EOM not yet received.
	stateReceiving = state(iota)

	// We received EOM from upstream and so have finished
	// receiving fragments. Still transmitting to downstream.
	// We have not yet acked the last message from upstream.
	stateReceivedEOM

	// We have sent the last fragment to downstream containing
	// the EOM flag. It has not yet been acked.
	stateSentEOM

	// The last fragment sent to downstream (with EOM set) has
	// been acknowledged. We can now acknowledge upstream's
	// last message and restart the sequence.
	stateEOMAcked
)

var (
	messageTooShort = errors.New("message too short to decode")
)

type reliableMessage struct {
	Flags    uint16
	Sequence uint32
	Payload  []byte
}

func (m *reliableMessage) MarshalBinary() ([]byte, error) {
	nbytes := reliableHeaderLength + len(m.Payload)
	var hdr [reliableHeaderLength]byte
	binary.BigEndian.PutUint16(hdr[0:2], m.Flags)
	binary.BigEndian.PutUint16(hdr[2:4], uint16(nbytes))
	binary.BigEndian.PutUint32(hdr[4:8], m.Sequence)
	result := append([]byte{}, hdr[:]...)
	result = append(result, m.Payload...)
	return result, nil
}

func (m *reliableMessage) UnmarshalBinary(data []byte) error {
	if len(data) < reliableHeaderLength {
		return messageTooShort
	}
	m.Flags = binary.BigEndian.Uint16(data[0:2])
	m.Sequence = binary.BigEndian.Uint32(data[4:8])
	m.Payload = append([]byte{}, data[8:]...)
	return nil
}

// reliableSharder receives Quake reliable message fragments, reassembles them
// into packets and retransmits them as fragments smaller than the MTU.
type reliableSharder struct {
	state        state
	rxseq, rxack uint32 // from upstream
	txseq, txack uint32 // to downstream
	txqueue      []byte
	txUpstream   func([]byte) error
	txDownstream func([]byte) error
}

func (s *reliableSharder) stateTransition(from, to state) {
	if s.state == from {
		s.state = to
	}
}

func (s *reliableSharder) sendUpstream(rm *reliableMessage) error {
	data, err := rm.MarshalBinary()
	if err != nil {
		return err
	}
	return s.txUpstream(data)
}

func (s *reliableSharder) sendDownstream(rm *reliableMessage) error {
	data, err := rm.MarshalBinary()
	if err != nil {
		return err
	}
	// TODO: Save and retransmit after timeout
	return s.txDownstream(data)
}

func (s *reliableSharder) sendNext() error {
	if s.txack != s.txseq {
		// Still waiting on ack of last packet
		return nil
	}
	if s.state == stateSentEOM || s.state == stateEOMAcked {
		// Nothing more to send yet
		return nil
	}
	if len(s.txqueue) == 0 {
		// Downstream has acked everything we sent so far, but we're
		// still waiting on upstream to send more fragments
		return nil
	}
	nbytes := vanillaQuakeMTU - reliableHeaderLength
	if nbytes > len(s.txqueue) {
		nbytes = len(s.txqueue)
	}
	rm := reliableMessage{
		Flags:    flagData,
		Sequence: s.txseq,
		Payload:  s.txqueue[:nbytes],
	}
	s.txqueue = s.txqueue[nbytes:]
	if s.state == stateReceivedEOM && len(s.txqueue) == 0 {
		rm.Flags |= flagEOM
		s.stateTransition(stateReceivedEOM, stateSentEOM)
	}
	s.txseq++
	return s.sendDownstream(&rm)
}

func (s *reliableSharder) sendAck() error {
	if s.rxseq == s.rxack {
		return nil
	}
	err := s.sendUpstream(&reliableMessage{
		Flags:    flagAck,
		Sequence: s.rxseq - 1,
	})
	s.rxack = s.rxseq
	return err
}

// receiveUpstream processes a packet received from the upstream
// Quake server and returns true, nil if the packet was handled.
func (s *reliableSharder) receiveUpstream(msg []byte) (bool, error) {
	flags := binary.BigEndian.Uint16(msg[0:2])
	if (flags & flagUnreliable) != 0 {
		return false, nil
	}
	if (flags & flagData) == 0 {
		// Reliable stream going the other way; not our responsibility.
		return false, nil
	}
	var rm reliableMessage
	if err := rm.UnmarshalBinary(msg); err != nil {
		return false, err
	}
	// We have received a reliable data fragment from upstream.
	if rm.Sequence == s.rxseq {
		s.stateTransition(stateEOMAcked, stateReceiving)
		s.txqueue = append(s.txqueue, rm.Payload...)
		s.rxseq++
		if (flags & flagEOM) != 0 {
			s.stateTransition(stateReceiving, stateReceivedEOM)
		}
	}

	// We don't acknowledge EOM until we got an ack from
	// downstream of our own EOM.
	if s.state == stateReceiving || s.state == stateEOMAcked {
		if err := s.sendAck(); err != nil {
			return false, err
		}
	}
	if err := s.sendNext(); err != nil {
		return false, err
	}

	return true, nil
}

// receiveDownstream processes a packet received from the downstream
// Quake client and returns true, nil if the packet was handled.
func (s *reliableSharder) receiveDownstream(msg []byte) (bool, error) {
	flags := binary.BigEndian.Uint16(msg[0:2])
	if (flags & flagUnreliable) != 0 {
		return false, nil
	}
	if (flags & flagAck) == 0 {
		// Reliable stream going the other way
		return false, nil
	}
	var rm reliableMessage
	if err := rm.UnmarshalBinary(msg); err != nil {
		return false, err
	}

	// We have received an ack from downstream.
	if rm.Sequence == s.txack {
		s.txack++
		s.stateTransition(stateSentEOM, stateEOMAcked)
		// Downstream acked EOM? We can ack upstream now
		var err error
		if s.state == stateEOMAcked {
			err = s.sendAck()
		} else {
			err = s.sendNext()
		}
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

func (s *reliableSharder) init(txUpstream, txDownstream func([]byte) error) {
	s.state = stateReceiving
	s.txqueue = []byte{}
	s.txUpstream = txUpstream
	s.txDownstream = txDownstream
}
