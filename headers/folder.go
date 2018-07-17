package headers

import (
	"io"
)

// Folder is a structure containing information on how a solid block was
// constructed.
type Folder struct {
	CoderInfo     []*CoderInfo
	BindPairsInfo []*BindPairsInfo
	PackedIndices []int
	UnpackSizes   []uint64
	UnpackCRC     uint32
}

// NumInStreamsTotal is the sum of inputs required by all codecs.
func (f *Folder) NumInStreamsTotal() int {
	var count int
	for i := range f.CoderInfo {
		count += f.CoderInfo[i].NumInStreams
	}
	return count
}

// NumOutStreamsTotal is the sum of outputs required by all codecs.
func (f *Folder) NumOutStreamsTotal() int {
	var count int
	for i := range f.CoderInfo {
		count += f.CoderInfo[i].NumOutStreams
	}
	return count
}

// FindBindPairForInStream returns the index of a bindpair by an in index.
func (f *Folder) FindBindPairForInStream(inStreamIndex int) int {
	for i := range f.BindPairsInfo {
		if f.BindPairsInfo[i].InIndex == inStreamIndex {
			return i
		}
	}
	return -1
}

// FindBindPairForOutStream returns the index of a bindpair by an out index.
func (f *Folder) FindBindPairForOutStream(outStreamIndex int) int {
	for i := range f.BindPairsInfo {
		if f.BindPairsInfo[i].OutIndex == outStreamIndex {
			return i
		}
	}
	return -1
}

// UnpackSize returns the final unpacked size of the folder.
func (f *Folder) UnpackSize() uint64 {
	for i := range f.UnpackSizes {
		if f.FindBindPairForOutStream(i) < 0 {
			return f.UnpackSizes[i]
		}
	}
	return 0
}

// ReadFolder reads a folder structure.
func ReadFolder(r io.Reader) (*Folder, error) {
	var err error

	folder := &Folder{}

	numCoders, err := ReadNumberInt(r)
	if err != nil {
		return nil, err
	}

	folder.CoderInfo = make([]*CoderInfo, numCoders)
	for i := range folder.CoderInfo {
		if folder.CoderInfo[i], err = ReadCoderInfo(r); err != nil {
			return nil, err
		}
	}

	folder.BindPairsInfo = make([]*BindPairsInfo, folder.NumOutStreamsTotal()-1)
	for i := range folder.BindPairsInfo {
		if folder.BindPairsInfo[i], err = ReadBindPairsInfo(r); err != nil {
			return nil, err
		}
	}

	numInStreamsTotal := folder.NumInStreamsTotal()
	numPackedStreams := numInStreamsTotal - len(folder.BindPairsInfo)
	if numPackedStreams > 1 {
		folder.PackedIndices = make([]int, numPackedStreams)
		for i := range folder.PackedIndices {
			if folder.PackedIndices[i], err = ReadNumberInt(r); err != nil {
				return nil, err
			}
		}
	} else if numPackedStreams == 1 {
		for i := 0; i < numInStreamsTotal; i++ {
			if folder.FindBindPairForInStream(i) < 0 {
				folder.PackedIndices = []int{i}
				break
			}
		}
	}

	return folder, nil
}

// CoderInfo is a structure holding information about a codec.
type CoderInfo struct {
	CodecID       uint32
	Properties    []byte
	NumInStreams  int
	NumOutStreams int
}

// ReadCoderInfo reads a coder info structure.
func ReadCoderInfo(r io.Reader) (*CoderInfo, error) {
	attributes, err := ReadByte(r)
	if err != nil {
		return nil, err
	}

	coderInfo := &CoderInfo{}

	codecIDSize := attributes & 0x0f
	isComplexCoder := attributes&0x10 > 0
	hasAttributes := attributes&0x20 > 0

	b := make([]byte, codecIDSize)
	if _, err = r.Read(b); err != nil {
		return nil, err
	}
	for i := codecIDSize; i > 0; i-- {
		coderInfo.CodecID |= uint32(b[i-1]) << ((codecIDSize - i) * 8)
	}

	coderInfo.NumInStreams = 1
	coderInfo.NumOutStreams = 1
	if isComplexCoder {
		if coderInfo.NumInStreams, err = ReadNumberInt(r); err != nil {
			return nil, err
		}
		if coderInfo.NumOutStreams, err = ReadNumberInt(r); err != nil {
			return nil, err
		}
	}

	if hasAttributes {
		size, err := ReadNumberInt(r)
		if err != nil {
			return nil, err
		}

		coderInfo.Properties = make([]byte, size)
		if _, err = r.Read(coderInfo.Properties); err != nil {
			return nil, err
		}
	}

	return coderInfo, nil
}

// BindPairsInfo is a structure that binds the in and out indexes of a codec.
type BindPairsInfo struct {
	InIndex  int
	OutIndex int
}

// ReadBindPairsInfo reads a bindpairs info structure.
func ReadBindPairsInfo(r io.Reader) (*BindPairsInfo, error) {
	bindPairsInfo := &BindPairsInfo{}

	var err error
	if bindPairsInfo.InIndex, err = ReadNumberInt(r); err != nil {
		return nil, err
	}
	if bindPairsInfo.OutIndex, err = ReadNumberInt(r); err != nil {
		return nil, err
	}

	return bindPairsInfo, nil
}
