package headers

import "io"

// UnpackInfo is a structure containing folders.
type UnpackInfo struct {
	Folders []*Folder
}

// ReadUnpackInfo reads unpack info structures.
func ReadUnpackInfo(r io.Reader) (*UnpackInfo, error) {
	err := ReadByteExpect(r, k7zFolder)
	if err != nil {
		return nil, err
	}

	numFolders, err := ReadNumberInt(r)
	if err != nil {
		return nil, err
	}

	unpackInfo := &UnpackInfo{}
	external, err := ReadByte(r)
	if err != nil {
		return nil, err
	}

	switch external {
	case 0:
		unpackInfo.Folders = make([]*Folder, numFolders)
		for i := range unpackInfo.Folders {
			if unpackInfo.Folders[i], err = ReadFolder(r); err != nil {
				return nil, err
			}
		}

	default:
		return nil, ErrAdditionalStreamsNotImplemented
	}

	if err = ReadByteExpect(r, k7zCodersUnpackSize); err != nil {
		return nil, err
	}
	for _, folder := range unpackInfo.Folders {
		folder.UnpackSizes = make([]uint64, folder.NumOutStreamsTotal())
		for i := range folder.UnpackSizes {
			if folder.UnpackSizes[i], err = ReadNumber(r); err != nil {
				return nil, err
			}
		}
	}

	id, err := ReadByte(r)
	if err != nil {
		return nil, err
	}
	if id == k7zCRC {
		crcs, err := ReadDigests(r, len(unpackInfo.Folders))
		if err != nil {
			return nil, err
		}
		for i := range unpackInfo.Folders {
			unpackInfo.Folders[i].UnpackCRC = crcs[i]
		}

		id, err = ReadByte(r)
		if err != nil {
			return nil, err
		}
	}

	if id != k7zEnd {
		return nil, ErrUnexpectedPropertyID
	}

	return unpackInfo, nil
}
