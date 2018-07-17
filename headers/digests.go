package headers

import (
	"encoding/binary"
	"io"
)

// ReadDigests reads an array of uint32 CRCs.
func ReadDigests(r io.Reader, length int) ([]uint32, error) {
	defined, _, err := ReadOptionalBoolVector(r, length)
	if err != nil {
		return nil, err
	}

	crcs := make([]uint32, length)
	for i := range defined {
		if defined[i] {
			if err := binary.Read(r, binary.LittleEndian, &crcs[i]); err != nil {
				return nil, err
			}
		}
	}

	return crcs, nil
}
