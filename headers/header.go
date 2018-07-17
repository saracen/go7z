package headers

import (
	"encoding/binary"
	"hash/crc32"
	"io"
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
	var raw [32]byte
	_, err := r.Read(raw[:])
	if err != nil {
		return nil, err
	}

	var header *SignatureHeader
	copy(header.Signature[:], raw[:6])
	header.ArchiveVersion.Major = raw[7]
	header.ArchiveVersion.Minor = raw[8]
	header.StartHeaderCRC = binary.LittleEndian.Uint32(raw[9:])
	header.StartHeader.NextHeaderOffset = int64(binary.LittleEndian.Uint64(raw[13:]))
	header.StartHeader.NextHeaderSize = int64(binary.LittleEndian.Uint64(raw[17:]))
	header.StartHeader.NextHeaderCRC = binary.LittleEndian.Uint32(raw[21:])

	if crc32.ChecksumIEEE(raw[12:]) != header.StartHeaderCRC {
		return header, ErrChecksumMismatch
	}
	return header, nil
}

// Header is structure containing file and stream information.
type Header struct {
	MainStreamsInfo *StreamsInfo
	FilesInfo       []*FileInfo
}

// ReadPackedStreamsForHeaders reads either a header or encoded header structure.
func ReadPackedStreamsForHeaders(r io.Reader) (header *Header, encodedHeader *StreamsInfo, err error) {
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
		break

	default:
		return nil, nil, ErrUnexpectedPropertyID
	}

	return header, encodedHeader, nil
}

// ReadHeader reads a header structure.
func ReadHeader(r io.Reader) (*Header, error) {
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
			if header.FilesInfo, err = ReadFilesInfo(r); err != nil {
				return nil, err
			}

		case k7zEnd:
			return header, nil

		default:
			return nil, ErrUnexpectedPropertyID
		}
	}
}
