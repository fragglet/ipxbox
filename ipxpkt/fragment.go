package ipxpkt

const (
	// TODO: Determine the correct maximum value to use here.
	maxFragmentPayload = 400
)

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
