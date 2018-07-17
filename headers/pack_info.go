package headers

import "io"

// PackInfo contains the pack stream sizes of the folders.
type PackInfo struct {
	PackPos   uint64
	PackSizes []uint64
}

// ReadPackInfo reads a pack info structure.
func ReadPackInfo(r io.Reader) (*PackInfo, error) {
	packInfo := &PackInfo{}

	var err error
	if packInfo.PackPos, err = ReadNumber(r); err != nil {
		return nil, err
	}

	numPackStreams, err := ReadNumberInt(r)
	if err != nil {
		return nil, err
	}

	for {
		id, err := ReadByte(r)
		if err != nil {
			return nil, err
		}

		switch id {
		case k7zSize:
			packInfo.PackSizes = make([]uint64, numPackStreams+1)
			for i := 0; i < numPackStreams; i++ {
				packInfo.PackSizes[i], err = ReadNumber(r)
				if err != nil {
					return nil, err
				}
			}

		case k7zCRC:
			return nil, ErrPackInfoCRCsNotImplemented

		case k7zEnd:
			return packInfo, nil

		default:
			return nil, ErrUnexpectedPropertyID
		}
	}
}
