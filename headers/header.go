package headers

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
)

const (
	// SignatureHeader size is the size of the signature header.
	SignatureHeaderSize = 32

	// MaxHeaderSize is the maximum header size.
	MaxHeaderSize = int64(1 << 62) // 4 exbibyte
)

var (
	// MagicBytes is the magic bytes used in the 7z signature.
	MagicBytes = [6]byte{0x37, 0x7A, 0xBC, 0xAF, 0x27, 0x1C}

	// ErrInvalidSignatureHeader is returned when signature header is invalid.
	ErrInvalidSignatureHeader = errors.New("invalid signature header")
)

// SignatureHeader is the structure found at the top of 7z files.
type SignatureHeader struct {
	Signature [6]byte

	ArchiveVersion struct {
		Major byte
		Minor byte
	}

	StartHeaderCRC uint32

	StartHeader struct {
		NextHeaderOffset int64
		NextHeaderSize   int64
		NextHeaderCRC    uint32
	}
}

// ReadSignatureHeader reads the signature header.
func ReadSignatureHeader(r io.Reader) (*SignatureHeader, error) {
	var raw [SignatureHeaderSize]byte
	_, err := r.Read(raw[:])
	if err != nil {
		return nil, err
	}

	var header SignatureHeader
	copy(header.Signature[:], raw[:6])
	if bytes.Compare(header.Signature[:], MagicBytes[:]) != 0 {
		return nil, ErrInvalidSignatureHeader
	}

	header.ArchiveVersion.Major = raw[6]
	header.ArchiveVersion.Minor = raw[7]
	header.StartHeaderCRC = binary.LittleEndian.Uint32(raw[8:])
	header.StartHeader.NextHeaderOffset = int64(binary.LittleEndian.Uint64(raw[12:]))
	header.StartHeader.NextHeaderSize = int64(binary.LittleEndian.Uint64(raw[20:]))
	header.StartHeader.NextHeaderCRC = binary.LittleEndian.Uint32(raw[28:])

	if header.StartHeader.NextHeaderSize < 0 || header.StartHeader.NextHeaderSize > MaxHeaderSize {
		return &header, ErrInvalidSignatureHeader
	}
	if crc32.ChecksumIEEE(raw[12:]) != header.StartHeaderCRC {
		err = ErrChecksumMismatch
	}
	return &header, err
}

// Header is structure containing file and stream information.
type Header struct {
	MainStreamsInfo *StreamsInfo
	FilesInfo       []*FileInfo
}

// ReadPackedStreamsForHeaders reads either a header or encoded header structure.
func ReadPackedStreamsForHeaders(r *io.LimitedReader) (header *Header, encodedHeader *StreamsInfo, err error) {
	id, err := ReadByte(r)
	if err != nil {
		return nil, nil, err
	}

	switch id {
	case k7zHeader:
		if header, err = ReadHeader(r); err != nil && err != io.EOF {
			return nil, nil, err
		}

	case k7zEncodedHeader:
		if encodedHeader, err = ReadStreamsInfo(r); err != nil {
			return nil, nil, err
		}

	case k7zEnd:
		if header == nil && encodedHeader == nil {
			return nil, nil, ErrUnexpectedPropertyID
		}
		break

	default:
		return nil, nil, ErrUnexpectedPropertyID
	}

	return header, encodedHeader, nil
}

// ReadHeader reads a header structure.
func ReadHeader(r *io.LimitedReader) (*Header, error) {
	header := &Header{}

	for {
		id, err := ReadByte(r)
		if err != nil {
			return nil, err
		}

		switch id {
		case k7zArchiveProperties:
			return nil, ErrArchivePropertiesNotImplemented

		case k7zAdditionalStreamsInfo:
			return nil, ErrAdditionalStreamsNotImplemented

		case k7zMainStreamsInfo:
			if header.MainStreamsInfo, err = ReadStreamsInfo(r); err != nil {
				return nil, err
			}

		case k7zFilesInfo:
			// Limit the maximum amount of FileInfos that get allocated to size
			// of the remaining header / 3
			if header.FilesInfo, err = ReadFilesInfo(r, int(r.N)/3); err != nil {
				return nil, err
			}

		case k7zEnd:
			if header.MainStreamsInfo == nil {
				return nil, ErrUnexpectedPropertyID
			}

			return header, nil

		default:
			return nil, ErrUnexpectedPropertyID
		}
	}
}
