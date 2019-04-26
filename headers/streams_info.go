package headers

import (
	"io"
)

// StreamsInfo is a top-level structure of the 7z format.
type StreamsInfo struct {
	PackInfo       *PackInfo
	UnpackInfo     *UnpackInfo
	SubStreamsInfo *SubStreamsInfo
}

// ReadStreamsInfo reads the streams info structure.
func ReadStreamsInfo(r io.Reader) (*StreamsInfo, error) {
	streamsInfo := &StreamsInfo{}

	for {
		id, err := ReadByte(r)
		if err != nil {
			return nil, err
		}

		switch id {
		case k7zPackInfo:
			if streamsInfo.PackInfo, err = ReadPackInfo(r); err != nil {
				return nil, err
			}

		case k7zUnpackInfo:
			if streamsInfo.UnpackInfo, err = ReadUnpackInfo(r); err != nil {
				return nil, err
			}

		case k7zSubStreamsInfo:
			if streamsInfo.UnpackInfo == nil {
				return nil, ErrUnexpectedPropertyID
			}

			if streamsInfo.SubStreamsInfo, err = ReadSubStreamsInfo(r, streamsInfo.UnpackInfo); err != nil {
				return nil, err
			}

		case k7zEnd:
			if streamsInfo.PackInfo == nil || streamsInfo.UnpackInfo == nil {
				return nil, ErrUnexpectedPropertyID
			}

			return streamsInfo, nil

		default:
			return nil, ErrUnexpectedPropertyID
		}
	}
}

// SubStreamsInfo is a structure found within the StreamsInfo structure.
type SubStreamsInfo struct {
	NumUnpackStreamsInFolders []int
	UnpackSizes               []uint64
	Digests                   []uint32
}

// ReadSubStreamsInfo reads the substreams info structure.
func ReadSubStreamsInfo(r io.Reader, unpackInfo *UnpackInfo) (*SubStreamsInfo, error) {
	id, err := ReadByte(r)
	if err != nil {
		return nil, err
	}

	subStreamInfo := &SubStreamsInfo{}
	subStreamInfo.NumUnpackStreamsInFolders = make([]int, len(unpackInfo.Folders))
	for i := range subStreamInfo.NumUnpackStreamsInFolders {
		subStreamInfo.NumUnpackStreamsInFolders[i] = 1
	}

	if id == k7zNumUnpackStream {
		for i := range subStreamInfo.NumUnpackStreamsInFolders {
			if subStreamInfo.NumUnpackStreamsInFolders[i], err = ReadNumberInt(r); err != nil {
				return nil, err
			}
		}

		id, err = ReadByte(r)
		if err != nil {
			return nil, err
		}
	}

	for i := range unpackInfo.Folders {
		if subStreamInfo.NumUnpackStreamsInFolders[i] == 0 {
			continue
		}

		var sum uint64
		if id == k7zSize {
			for j := 1; j < subStreamInfo.NumUnpackStreamsInFolders[i]; j++ {
				size, err := ReadNumber(r)
				if err != nil {
					return nil, err
				}

				sum += size
				subStreamInfo.UnpackSizes = append(subStreamInfo.UnpackSizes, size)
			}
		}

		subStreamInfo.UnpackSizes = append(subStreamInfo.UnpackSizes, unpackInfo.Folders[i].UnpackSize()-uint64(sum))
	}

	if id == k7zSize {
		id, err = ReadByte(r)
		if err != nil {
			return nil, err
		}
	}

	numDigests := 0
	for i := range unpackInfo.Folders {
		numSubStreams := subStreamInfo.NumUnpackStreamsInFolders[i]
		if numSubStreams > 1 || unpackInfo.Folders[i].UnpackCRC == 0 {
			numDigests += int(numSubStreams)
		}
	}

	if id == k7zCRC {
		subStreamInfo.Digests, err = ReadDigests(r, numDigests)
		if err != nil {
			return nil, err
		}

		id, err = ReadByte(r)
		if err != nil {
			return nil, err
		}
	}

	if id != k7zEnd {
		return nil, ErrUnexpectedPropertyID
	}

	return subStreamInfo, nil
}
