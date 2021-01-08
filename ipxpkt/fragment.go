package ipxpkt

import (
	"time"

	"github.com/fragglet/ipxbox/ipx"
)

const (
	// TODO: Determine the correct maximum value to use here.
	maxFragmentPayload = 400

	// maxFrames is the maximum number of frames we store for reassembly
	// at any given time.
	maxFrames = 16

	// maxAge is the maximum amount of time that we hold a frame for
	// reassembly before giving up and flushing it.
	maxAge = 10 * time.Second
)

type frameKey struct {
	src      ipx.HeaderAddr
	packetID uint16
}

type frameData struct {
	fragments [][]byte
	lastRX    time.Time
}

type frameReassembler struct {
	frames map[frameKey]*frameData
}

func (fd *frameData) processFragment(hdr *Header, fragment []byte) ([]byte, bool) {
	// Sanity check first:
	if int(hdr.NumFragments) != len(fd.fragments) {
		return nil, false
	}
	fd.lastRX = time.Now()
	fd.fragments[hdr.Fragment-1] = append([]byte{}, fragment...)
	for _, f := range fd.fragments {
		if f == nil {
			return nil, false
		}
	}
	result := []byte{}
	for _, f := range fd.fragments {
		result = append(result, f...)
	}
	return result, true
}

func (fr *frameReassembler) init() {
	fr.frames = make(map[frameKey]*frameData)
}

// flush empties out old frames from the queue that are older than maxAge. If
// none are older than maxAge then the frame is emptied that has been sitting
// in the queue for the longest time with no fragment being received.
func (fr *frameReassembler) flush() {
	flushKeys := make([]frameKey, 0)
	now := time.Now()
	var oldest frameKey
	var oldestTime time.Time
	for key, data := range fr.frames {
		if now.After(data.lastRX.Add(maxAge)) {
			flushKeys = append(flushKeys, key)
		} else if oldestTime.IsZero() || data.lastRX.Before(oldestTime) {
			oldest = key
			oldestTime = data.lastRX
		}
	}
	for _, key := range flushKeys {
		delete(fr.frames, key)
	}
	// We always flush at least one frame from the queue to make space.
	if len(flushKeys) == 0 {
		delete(fr.frames, oldest)
	}
}

func (fr *frameReassembler) reassemble(ipxHeader *ipx.Header, hdr *Header, fragment []byte) ([]byte, bool) {
	// Simplest optimization, no reassembly required:
	if hdr.NumFragments == 1 {
		return fragment, true
	}
	key := frameKey{
		src:      ipxHeader.Src,
		packetID: hdr.PacketID,
	}
	fd, ok := fr.frames[key]
	// First fragment of frame?
	if !ok {
		if len(fr.frames) >= maxFrames {
			fr.flush()
		}
		fd = &frameData{
			fragments: make([][]byte, hdr.NumFragments),
		}
		fr.frames[key] = fd
	}
	result, ok := fd.processFragment(hdr, fragment)
	if !ok {
		return nil, false
	}
	delete(fr.frames, key)
	return result, true
}

// fragmentFrame breaks the packet in the given slice into one or more smaller
// slices, none of which is larger than maxFragmentPayload in length.
func fragmentFrame(frame []byte) [][]byte {
	numFragments := (len(frame) + maxFragmentPayload - 1) / maxFragmentPayload
	result := make([][]byte, numFragments)
	offset := 0
	for i := 0; i < numFragments; i++ {
		nextOffset := offset + maxFragmentPayload
		if nextOffset > len(frame) {
			nextOffset = len(frame)
		}
		result[i] = frame[offset:nextOffset]
		offset = nextOffset
	}
	return result
}
