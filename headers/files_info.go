package headers

import (
	"encoding/binary"
	"errors"
	"io"
	"time"
	"unicode/utf16"
)

// ErrInvalidFileCount is returned when the file count read from the stream
// exceeds the caller supplied maxFileCount.
var ErrInvalidFileCount = errors.New("invalid file count")

// FileInfo is a structure containing the information of an archived file.
type FileInfo struct {
	Name   string
	Attrib uint32

	IsEmptyStream bool
	IsEmptyFile   bool

	// Flag indicating a file should be removed upon extraction.
	IsAntiFile bool

	CreatedAt  time.Time
	AccessedAt time.Time
	ModifiedAt time.Time
}

// ReadFilesInfo reads the files info structure.
func ReadFilesInfo(r io.Reader, maxFileCount int) ([]*FileInfo, error) {
	numFiles, err := ReadNumberInt(r)
	if err != nil {
		return nil, err
	}
	if numFiles > maxFileCount {
		return nil, ErrInvalidFileCount
	}

	fileInfo := make([]*FileInfo, numFiles)
	for i := range fileInfo {
		fileInfo[i] = &FileInfo{}
	}

	var numEmptyStreams int
	for {
		id, err := ReadByte(r)
		if err != nil {
			return nil, err
		}

		if id == k7zEnd {
			return fileInfo, nil
		}

		size, err := ReadNumber(r)
		if err != nil {
			return nil, err
		}

		switch id {
		case k7zEmptyStream:
			var emptyStreams []bool
			emptyStreams, numEmptyStreams, err = ReadBoolVector(r, numFiles)
			if err != nil {
				return nil, err
			}
			for i, fi := range fileInfo {
				fi.IsEmptyStream = emptyStreams[i]
			}

		case k7zEmptyFile, k7zAnti:
			files, _, err := ReadBoolVector(r, numEmptyStreams)
			if err != nil {
				return nil, err
			}

			idx := 0
			for _, fi := range fileInfo {
				if fi.IsEmptyStream {
					switch id {
					case k7zEmptyFile:
						fi.IsEmptyFile = files[idx]
					case k7zAnti:
						fi.IsAntiFile = files[idx]
					}
					idx++
				}
			}

		case k7zStartPos:
			return nil, ErrUnexpectedPropertyID

		case k7zCTime, k7zATime, k7zMTime:
			times, err := ReadDateTimeVector(r, numFiles)
			if err != nil {
				return nil, err
			}
			for i, fi := range fileInfo {
				switch id {
				case k7zCTime:
					fi.CreatedAt = times[i]
				case k7zATime:
					fi.AccessedAt = times[i]
				case k7zMTime:
					fi.ModifiedAt = times[i]
				}
			}

		case k7zName:
			external, err := ReadByte(r)
			if err != nil {
				return nil, err
			}

			switch external {
			case 0:
				for _, fi := range fileInfo {
					var rune uint16
					var name []uint16
					for {
						if err = binary.Read(r, binary.LittleEndian, &rune); err != nil {
							return nil, err
						}

						if rune == 0 {
							break
						}
						name = append(name, rune)
					}
					fi.Name = string(utf16.Decode(name))
				}

			default:
				return nil, ErrAdditionalStreamsNotImplemented
			}

		case k7zWinAttributes:
			attributes, err := ReadAttributeVector(r, numFiles)
			if err != nil {
				return nil, err
			}
			for i, fi := range fileInfo {
				fi.Attrib = attributes[i]
			}

		case k7zDummy:
			for i := uint64(0); i < size; i++ {
				if _, err = ReadByte(r); err != nil {
					return nil, err
				}
			}

		default:
			return nil, ErrUnexpectedPropertyID
		}
	}
}
