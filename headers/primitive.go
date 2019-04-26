package headers

import (
	"encoding/binary"
	"errors"
	"io"
	"time"
)

const (
	k7zEnd = iota
	k7zHeader
	k7zArchiveProperties
	k7zAdditionalStreamsInfo
	k7zMainStreamsInfo
	k7zFilesInfo
	k7zPackInfo
	k7zUnpackInfo
	k7zSubStreamsInfo
	k7zSize
	k7zCRC
	k7zFolder
	k7zCodersUnpackSize
	k7zNumUnpackStream
	k7zEmptyStream
	k7zEmptyFile
	k7zAnti
	k7zName
	k7zCTime
	k7zATime
	k7zMTime
	k7zWinAttributes
	k7zComment
	k7zEncodedHeader
	k7zStartPos
	k7zDummy
)

const MaxNumber = 0x7FFFFFFF

var (
	// ErrUnexpectedPropertyID is returned when we read a property id that was
	// either unexpected, or we don't support.
	ErrUnexpectedPropertyID = errors.New("unexpected property id")

	// ErrAdditionalStreamsNotImplemented is returned for archives using
	// additional streams. These were apparently used in older versions of 7zip.
	ErrAdditionalStreamsNotImplemented = errors.New("additional streams are not implemented")

	// ErrArchivePropertiesNotImplemented is returned if archive properties
	// structure is found. So far, this hasn't been used in any verison of 7zip.
	ErrArchivePropertiesNotImplemented = errors.New("archive properties are not implemented")

	// ErrChecksumMismatch is returned when a CRC check fails.
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrPackInfoCRCsNotImplemented is returned if a CRC property id is
	// encountered whilst reading packinfo.
	ErrPackInfoCRCsNotImplemented = errors.New("packinfo crcs are not implemented")

	// ErrInvalidNumber is returned when a number read exceeds 0x7FFFFFFF
	ErrInvalidNumber = errors.New("invalid number")
)

// ReadByte reads a single byte.
func ReadByte(r io.Reader) (byte, error) {
	var val [1]byte
	_, err := r.Read(val[:])
	return val[0], err
}

// ReadByteExpect reads a byte to be expected, errors if unexpected.
func ReadByteExpect(r io.Reader, val byte) error {
	value, err := ReadByte(r)
	if err != nil {
		return err
	}
	if value != val {
		return ErrUnexpectedPropertyID
	}
	return nil
}

// ReadNumber reads a 7z encoded uint64.
func ReadNumber(r io.Reader) (uint64, error) {
	first, err := ReadByte(r)
	if err != nil {
		return 0, err
	}

	var value uint64
	mask := byte(0x80)
	for i := uint64(0); i < 8; i++ {
		if first&mask == 0 {
			hp := uint64(first) & (uint64(mask) - 1)
			value += hp << (i * 8)
			return value, nil
		}

		val, err := ReadByte(r)
		if err != nil {
			return 0, err
		}

		value |= uint64(val) << (8 * i)
		mask >>= 1
	}

	return value, nil
}

// ReadNumberInt is the same as ReadNumber, but cast to int.
func ReadNumberInt(r io.Reader) (int, error) {
	u64, err := ReadNumber(r)
	if u64 > MaxNumber {
		return 0, ErrInvalidNumber
	}

	return int(u64), err
}

// ReadUint32 reads a uint32.
func ReadUint32(r io.Reader) (uint32, error) {
	var v uint32
	return v, binary.Read(r, binary.LittleEndian, &v)
}

// ReadUint64 reads a uint64.
func ReadUint64(r io.Reader) (uint64, error) {
	var v uint64
	return v, binary.Read(r, binary.LittleEndian, &v)
}

// ReadBoolVector reads a vector of boolean values.
func ReadBoolVector(r io.Reader, length int) ([]bool, int, error) {
	var b byte
	var mask byte
	var err error
	v := make([]bool, length)

	count := 0
	for i := range v {
		if mask == 0 {
			b, err = ReadByte(r)
			if err != nil {
				return nil, 0, err
			}
			mask = 0x80
		}
		v[i] = (b & mask) != 0
		mask >>= 1
		if v[i] {
			count++
		}
	}

	return v, count, nil
}

// ReadOptionalBoolVector reads a vector of boolean values if they're available,
// otherwise it returns an array of booleans all being true.
func ReadOptionalBoolVector(r io.Reader, length int) ([]bool, int, error) {
	allDefined, err := ReadByte(r)
	if err != nil {
		return nil, 0, err
	}

	if allDefined == 0 {
		return ReadBoolVector(r, length)
	}

	defined := make([]bool, length)
	for i := range defined {
		defined[i] = true
	}

	return defined, length, nil
}

// ReadNumberVector returns a vector of 7z encoded int64s.
func ReadNumberVector(r io.Reader, numFiles int) ([]*int64, error) {
	defined, _, err := ReadOptionalBoolVector(r, numFiles)
	if err != nil {
		return nil, err
	}

	external, err := ReadByte(r)
	if err != nil {
		return nil, err
	}
	if external != 0 {
		return nil, ErrAdditionalStreamsNotImplemented
	}

	numbers := make([]*int64, numFiles)
	for i := 0; i < numFiles; i++ {
		if defined[i] {
			num, err := ReadUint64(r)
			if err != nil {
				return nil, err
			}

			val := int64(num)
			numbers[i] = &val
		} else {
			numbers[i] = nil
		}
	}

	return numbers, err
}

// ReadDateTimeVector reads a vector of datetime values.
func ReadDateTimeVector(r io.Reader, numFiles int) ([]time.Time, error) {
	timestamps, err := ReadNumberVector(r, numFiles)
	if err != nil {
		return nil, err
	}

	times := make([]time.Time, len(timestamps))
	for i := range times {
		if timestamps[i] != nil {
			nsec := *timestamps[i]
			nsec -= 116444736000000000
			nsec *= 100

			times[i] = time.Unix(0, nsec)
		}
	}

	return times, nil
}

// ReadAttributeVector reads a vector of uint32s.
func ReadAttributeVector(r io.Reader, numFiles int) ([]uint32, error) {
	defined, _, err := ReadOptionalBoolVector(r, numFiles)
	if err != nil {
		return nil, err
	}

	external, err := ReadByte(r)
	if err != nil {
		return nil, err
	}
	if external != 0 {
		return nil, ErrAdditionalStreamsNotImplemented
	}

	attributes := make([]uint32, numFiles)
	for i := range attributes {
		if defined[i] {
			val, err := ReadUint32(r)
			if err != nil {
				return nil, err
			}

			attributes[i] = val
		}
	}

	return attributes, nil
}
