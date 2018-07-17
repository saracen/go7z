package go7z

import (
	"bufio"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"os"

	"github.com/saracen/go7z/headers"
	"github.com/saracen/solidblock"
)

var (
	// ErrNotSupported is returned when an unrecognized archive format is
	// encountered.
	ErrNotSupported = errors.New("not supported")

	// ErrDecompressorNotFound is returned when a requested decompressor has not
	// been registered.
	ErrDecompressorNotFound = errors.New("decompressor not found")
)

// Reader is a 7z archive reader.
type Reader struct {
	r   *io.SectionReader
	err error

	header *headers.Header

	folderIndex int
	fileIndex   int
	emptyStream bool

	solidblocks []*solidblock.Solidblock
}

// ReadCloser provides an io.ReadCloser for the archive when opened with
// OpenReader.
type ReadCloser struct {
	f *os.File
	Reader
}

// Close closes the 7z file, rendering it unusable for I/O.
func (rc *ReadCloser) Close() error {
	return rc.f.Close()
}

// OpenReader will open the 7z file specified by name and return a ReadCloser.
func OpenReader(name string) (*ReadCloser, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}

	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	r := new(ReadCloser)
	if err := r.init(f, fi.Size()); err != nil {
		f.Close()
		return nil, err
	}
	r.f = f

	return r, nil
}

// NewReader returns a new Reader reading from r, which is assumed to
// have the given size in bytes.
func NewReader(r io.ReaderAt, size int64) (*Reader, error) {
	szr := new(Reader)
	if err := szr.init(r, size); err != nil {
		return nil, err
	}
	return szr, nil
}

func (sz *Reader) init(r io.ReaderAt, size int64) error {
	var signatureHeader headers.SignatureHeader

	sz.r = io.NewSectionReader(r, 0, size)

	if err := binary.Read(sz.r, binary.LittleEndian, &signatureHeader); err != nil {
		return err
	}
	if _, err := sz.r.Seek(signatureHeader.StartHeader.NextHeaderOffset, 1); err != nil {
		return err
	}

	crc := crc32.NewIEEE()
	tee := io.TeeReader(io.LimitReader(bufio.NewReader(sz.r), signatureHeader.StartHeader.NextHeaderSize), crc)

	header, encoded, err := headers.ReadPackedStreamsForHeaders(tee)
	if err != nil {
		return err
	}
	if crc.Sum32() != signatureHeader.StartHeader.NextHeaderCRC {
		return headers.ErrChecksumMismatch
	}

	if encoded != nil {
		solidblocks, err := extract(sz.r, encoded)
		if err != nil {
			return err
		}
		if len(solidblocks) != 1 {
			return ErrNotSupported
		}
		if err = solidblocks[0].Next(); err != nil {
			return err
		}

		header, _, err = headers.ReadPackedStreamsForHeaders(solidblocks[0])
		if err != nil {
			return err
		}
		if header == nil {
			return ErrNotSupported
		}
		if err = solidblocks[0].Next(); err != io.EOF {
			return ErrNotSupported
		}
	}

	sz.header = header
	sz.solidblocks, err = extract(sz.r, sz.header.MainStreamsInfo)

	return err
}

// Next advances to the next entry in the 7z archive.
//
// io.EOF is returned at the end of the input.
func (sz *Reader) Next() (*headers.FileInfo, error) {
	if sz.err != nil {
		return nil, sz.err
	}
	hdr, err := sz.next()
	sz.err = err
	return hdr, err
}

func (sz *Reader) nextFileInfo() *headers.FileInfo {
	var fileInfo *headers.FileInfo
	if sz.fileIndex < len(sz.header.FilesInfo) {
		fileInfo = sz.header.FilesInfo[sz.fileIndex]
		sz.fileIndex++
		return fileInfo
	}

	return nil
}

func extract(r io.ReaderAt, streamsInfo *headers.StreamsInfo) ([]*solidblock.Solidblock, error) {
	var sizes []uint64
	var crcs []uint32
	if streamsInfo.SubStreamsInfo != nil {
		sizes = streamsInfo.SubStreamsInfo.UnpackSizes
		crcs = streamsInfo.SubStreamsInfo.Digests
	}

	offset := int64(32)
	offset += int64(streamsInfo.PackInfo.PackPos)
	packedIndicesOffset := 0

	var solidblocks []*solidblock.Solidblock
	for i, folder := range streamsInfo.UnpackInfo.Folders {
		if len(folder.PackedIndices) == 0 {
			folder.PackedIndices = []int{0}
		}

		binder := solidblock.Binder{}

		// setup codecs
		for j := range folder.CoderInfo {
			coderInfo := folder.CoderInfo[j]
			size := folder.UnpackSizes[j]

			d := decompressor(coderInfo.CodecID)
			if d == nil {
				return solidblocks, ErrDecompressorNotFound
			}

			fn := func(in []io.Reader) ([]io.Reader, error) {
				r, err := d(in, coderInfo.Properties, size)

				return []io.Reader{r}, err
			}

			binder.AddCodec(fn, coderInfo.NumInStreams, coderInfo.NumOutStreams)
		}

		// setup initial inputs
		for index, input := range folder.PackedIndices {
			size := int64(streamsInfo.PackInfo.PackSizes[packedIndicesOffset+index])
			binder.Reader(bufio.NewReader(io.NewSectionReader(r, offset, size)), input)
			offset += size
		}
		packedIndicesOffset += len(folder.PackedIndices)

		// setup pairs
		for _, bindPairsInfo := range folder.BindPairsInfo {
			binder.Pair(bindPairsInfo.InIndex, bindPairsInfo.OutIndex)
		}

		outputs, err := binder.Outputs()
		if err != nil {
			return solidblocks, err
		}

		if len(outputs) != 1 {
			return solidblocks, ErrNotSupported
		}

		var sizesInFolder []uint64
		var crcsInFolder []uint32
		if streamsInfo.SubStreamsInfo != nil {
			sizesInFolder = sizes[:streamsInfo.SubStreamsInfo.NumUnpackStreamsInFolders[i]]
			crcsInFolder = crcs[:streamsInfo.SubStreamsInfo.NumUnpackStreamsInFolders[i]]
			sizes = sizes[len(sizesInFolder):]
			crcs = crcs[len(crcsInFolder):]
		} else {
			sizesInFolder = []uint64{folder.UnpackSize()}
			crcsInFolder = []uint32{folder.UnpackCRC}
		}

		solidblocks = append(solidblocks, solidblock.New(outputs[0], sizesInFolder, crcsInFolder))
	}

	return solidblocks, nil
}

func (sz *Reader) next() (*headers.FileInfo, error) {
	fileInfo := sz.nextFileInfo()
	if fileInfo == nil {
		return nil, io.EOF
	}

	sz.emptyStream = fileInfo.IsEmptyStream
	if sz.emptyStream {
		return fileInfo, nil
	}

	if sz.solidblocks[sz.folderIndex].Next() == io.EOF {
		sz.folderIndex++
		sz.solidblocks[sz.folderIndex].Next()
	}

	return fileInfo, nil
}

// Read reads from the current file in the 7z archive.
// It returns (0, io.EOF) when it reaches the end of that file,
// until Next is called to advance to the next file.
func (sz *Reader) Read(p []byte) (int, error) {
	if sz.err != nil {
		return 0, sz.err
	}
	if sz.emptyStream {
		return 0, io.EOF
	}

	n, err := sz.solidblocks[sz.folderIndex].Read(p)
	if err != nil && err != io.EOF {
		sz.err = err
	}
	return n, err
}
