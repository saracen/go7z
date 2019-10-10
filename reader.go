package go7z

import (
	"bufio"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sync"

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

	folders []*folderReader

	Options ReaderOptions
}

// ReaderOptions are optional options to configure a 7z archive reader.
type ReaderOptions struct {
	password string
	cb       func() string
}

// SetPassword sets the password used for extraction.
func (o *ReaderOptions) SetPassword(password string) {
	o.password = password
}

// SetPasswordCallback sets the callback thats used if a password is required,
// but wasn't supplied with SetPassword()
func (o *ReaderOptions) SetPasswordCallback(cb func() string) {
	o.cb = cb
}

// Password returns the set password. This will call the password callback
// supplied to SetPasswordCallback() if no password is set.
func (o *ReaderOptions) Password() string {
	if o.password != "" {
		return o.password
	}
	if o.cb != nil {
		o.password = o.cb()
	}
	return o.password
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
	if err := r.init(f, fi.Size(), false); err != nil {
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
	if err := szr.init(r, size, false); err != nil {
		return nil, err
	}
	return szr, nil
}

func (sz *Reader) init(r io.ReaderAt, size int64, ignoreChecksumError bool) error {
	sz.r = io.NewSectionReader(r, 0, size)
	signatureHeader, err := headers.ReadSignatureHeader(sz.r)
	if err != nil {
		if !(ignoreChecksumError && err == headers.ErrChecksumMismatch) {
			return err
		}
	}
	if _, err := sz.r.Seek(signatureHeader.StartHeader.NextHeaderOffset, io.SeekCurrent); err != nil {
		return err
	}

	if signatureHeader.StartHeader.NextHeaderSize > size-headers.SignatureHeaderSize {
		return io.ErrUnexpectedEOF
	}

	crc := crc32.NewIEEE()
	tee := io.TeeReader(bufio.NewReader(io.LimitReader(sz.r, signatureHeader.StartHeader.NextHeaderSize)), crc)

	header, encoded, err := headers.ReadPackedStreamsForHeaders(&io.LimitedReader{tee, signatureHeader.StartHeader.NextHeaderSize})
	if err != nil {
		return err
	}
	if crc.Sum32() != signatureHeader.StartHeader.NextHeaderCRC {
		if !ignoreChecksumError {
			return headers.ErrChecksumMismatch
		}
	}

	if encoded != nil {
		folders, err := sz.extract(encoded)
		if err != nil {
			return err
		}
		if len(folders) != 1 {
			return ErrNotSupported
		}
		if err = folders[0].Next(); err != nil {
			return err
		}

		header, _, err = headers.ReadPackedStreamsForHeaders(&io.LimitedReader{folders[0].sb, folders[0].sb.Size()})
		if err != nil {
			return err
		}

		if err = folders[0].Next(); err != io.EOF {
			return ErrNotSupported
		}
	}

	if header == nil {
		return ErrNotSupported
	}
	sz.header = header
	sz.folders, err = sz.extract(sz.header.MainStreamsInfo)

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

func (sz *Reader) extract(streamsInfo *headers.StreamsInfo) ([]*folderReader, error) {
	var sizes []uint64
	var crcs []uint32
	if streamsInfo.SubStreamsInfo != nil {
		sizes = streamsInfo.SubStreamsInfo.UnpackSizes
		crcs = streamsInfo.SubStreamsInfo.Digests
	}

	offset := int64(headers.SignatureHeaderSize)
	offset += int64(streamsInfo.PackInfo.PackPos)
	packedIndicesOffset := 0

	var folders []*folderReader
	for i, folder := range streamsInfo.UnpackInfo.Folders {
		if len(folder.PackedIndices) == 0 {
			folder.PackedIndices = []int{0}
		}

		fr := &folderReader{}
		fr.inputs = make(map[int]io.Reader)
		fr.binder = solidblock.Binder{}

		// setup codecs
		for j := range folder.CoderInfo {
			coderInfo := folder.CoderInfo[j]
			size := folder.UnpackSizes[j]

			d := decompressor(coderInfo.CodecID)
			if d == nil {
				return folders, ErrDecompressorNotFound
			}

			fn := func(in []io.Reader) ([]io.Reader, error) {
				r, err := d(in, coderInfo.Properties, size, &sz.Options)

				return []io.Reader{r}, err
			}

			fr.binder.AddCodec(fn, coderInfo.NumInStreams, coderInfo.NumOutStreams)
		}

		// setup initial inputs
		for index, input := range folder.PackedIndices {
			if packedIndicesOffset+index >= len(streamsInfo.PackInfo.PackSizes) {
				return nil, fmt.Errorf("folder references invalid packinfo")
			}

			size := int64(streamsInfo.PackInfo.PackSizes[packedIndicesOffset+index])
			fr.inputs[input] = io.NewSectionReader(sz.r, offset, size)
			offset += size
		}
		packedIndicesOffset += len(folder.PackedIndices)

		// setup pairs
		for _, bindPairsInfo := range folder.BindPairsInfo {
			fr.binder.Pair(bindPairsInfo.InIndex, bindPairsInfo.OutIndex)
		}

		if streamsInfo.SubStreamsInfo != nil {
			numUnpackStreamsInFolders := streamsInfo.SubStreamsInfo.NumUnpackStreamsInFolders
			if i >= len(numUnpackStreamsInFolders) {
				return nil, fmt.Errorf("folder references invalid unpack stream")
			}

			off := numUnpackStreamsInFolders[i]
			if off > len(sizes) || off > len(crcs) {
				return nil, fmt.Errorf("folder references invalid unpack size or digest")
			}

			fr.sizes = sizes[:off]
			fr.crcs = crcs[:off]
			sizes = sizes[len(fr.sizes):]
			crcs = crcs[len(fr.crcs):]
		} else {
			fr.sizes = []uint64{folder.UnpackSize()}
			fr.crcs = []uint32{folder.UnpackCRC}
		}

		folders = append(folders, fr)
	}

	return folders, nil
}

type folderReader struct {
	binder solidblock.Binder
	sizes  []uint64
	crcs   []uint32

	inputs map[int]io.Reader

	bufs []*bufio.Reader

	sb *solidblock.Solidblock
}

var bufioReaderPool = sync.Pool{
	New: func() interface{} {
		return bufio.NewReaderSize(nil, 32*1024)
	},
}

func (fr *folderReader) Next() error {
	if fr.sb == nil {

		fr.bufs = make([]*bufio.Reader, 0, len(fr.inputs))
		for in, r := range fr.inputs {
			br := bufioReaderPool.Get().(*bufio.Reader)
			br.Reset(r)
			fr.bufs = append(fr.bufs, br)

			fr.binder.Reader(br, in)
		}

		outputs, err := fr.binder.Outputs()
		if err != nil {
			return err
		}
		if len(outputs) != 1 {
			return ErrNotSupported
		}
		if outputs[0] == nil {
			return ErrNotSupported
		}

		fr.sb = solidblock.New(outputs[0], fr.sizes, fr.crcs)
	}

	return fr.sb.Next()
}

func (fr *folderReader) Close() error {
	for _, buf := range fr.bufs {
		bufioReaderPool.Put(buf)
	}
	fr.bufs = nil
	return nil
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

	if sz.folders[sz.folderIndex].Next() == io.EOF {
		sz.folders[sz.folderIndex].Close()
		sz.folderIndex++
		if sz.folderIndex >= len(sz.folders) {
			return nil, io.EOF
		}
		sz.folders[sz.folderIndex].Next()
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

	n, err := sz.folders[sz.folderIndex].sb.Read(p)
	if err != nil && err != io.EOF {
		sz.err = err
	}
	return n, err
}
