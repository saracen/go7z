package go7z

import (
	"bytes"
	"compress/bzip2"
	"compress/flate"
	"encoding/binary"
	"io"
	"sync"

	"github.com/saracen/go7z/filters"
	"github.com/ulikunitz/xz/lzma"
)

// Decompressor is a handler function called when a registered decompressor is
// initialized.
type Decompressor func(r []io.Reader, options []byte, unpackSize uint64, ro *ReaderOptions) (io.Reader, error)

var (
	decompressors sync.Map // map[uint32]Decompressor
)

func init() {
	// copy
	RegisterDecompressor(0x00, Decompressor(func(r []io.Reader, options []byte, unpackSize uint64, ro *ReaderOptions) (io.Reader, error) {
		if len(r) != 1 {
			return nil, ErrNotSupported
		}
		return r[0], nil
	}))

	// delta
	RegisterDecompressor(0x03, Decompressor(func(r []io.Reader, options []byte, unpackSize uint64, ro *ReaderOptions) (io.Reader, error) {
		if len(r) != 1 || len(options) == 0 || len(options) > 1 {
			return nil, ErrNotSupported
		}

		return filters.NewDeltaDecoder(r[0], uint(options[0])+1, int64(unpackSize))
	}))

	// lzma
	RegisterDecompressor(0x030101, Decompressor(func(r []io.Reader, options []byte, unpackSize uint64, ro *ReaderOptions) (io.Reader, error) {
		if len(r) != 1 {
			return nil, ErrNotSupported
		}

		// We can't set options in the lzma decoder library, so instead we add
		// a fake header
		header := bytes.NewBuffer(options)
		binary.Write(header, binary.LittleEndian, unpackSize)

		return lzma.NewReader(io.MultiReader(header, r[0]))
	}))

	// lzma2
	RegisterDecompressor(0x21, Decompressor(func(r []io.Reader, options []byte, unpackSize uint64, ro *ReaderOptions) (io.Reader, error) {
		if len(r) != 1 {
			return nil, ErrNotSupported
		}

		config := lzma.Reader2Config{}
		if len(options) > 0 {
			config.DictCap = int(2 | (options[0] & 1))
			config.DictCap <<= (options[0] >> 1) + 11
		}

		return config.NewReader2(r[0])
	}))

	// bcj2
	RegisterDecompressor(0x303011b, Decompressor(func(r []io.Reader, options []byte, unpackSize uint64, ro *ReaderOptions) (io.Reader, error) {
		if len(r) != 4 {
			return nil, ErrNotSupported
		}
		return filters.NewBCJ2Decoder(r[0], r[1], r[2], r[3], int64(unpackSize))
	}))

	// deflate
	RegisterDecompressor(0x40108, Decompressor(func(r []io.Reader, options []byte, unpackSize uint64, ro *ReaderOptions) (io.Reader, error) {
		if len(r) != 1 {
			return nil, ErrNotSupported
		}
		return flate.NewReader(r[0]), nil
	}))

	// bzip2
	RegisterDecompressor(0x40202, Decompressor(func(r []io.Reader, options []byte, unpackSize uint64, ro *ReaderOptions) (io.Reader, error) {
		if len(r) != 1 {
			return nil, ErrNotSupported
		}

		return bzip2.NewReader(r[0]), nil
	}))

	// AES
	RegisterDecompressor(0x6f10701, Decompressor(func(r []io.Reader, options []byte, unpackSize uint64, ro *ReaderOptions) (io.Reader, error) {
		if len(r) != 1 {
			return nil, ErrNotSupported
		}
		if len(options) < 2 {
			return nil, ErrNotSupported
		}

		saltSize := ((options[0] >> 7) & 1) + (options[1] >> 4)
		ivSize := ((options[0] >> 6) & 1) + (options[1] & 0x0F)
		power := int(options[0]) & 0x3f

		options = options[2:]
		salt := options[:saltSize]
		iv := options[saltSize : saltSize+ivSize]

		return filters.NewAESDecrypter(r[0], power, salt, iv, ro.Password())
	}))
}

// RegisterDecompressor registers a decompressor.
func RegisterDecompressor(method uint32, dcomp Decompressor) {
	if _, dup := decompressors.LoadOrStore(method, dcomp); dup {
		panic("decompressor already registered")
	}
}

func decompressor(method uint32) Decompressor {
	di, ok := decompressors.Load(method)
	if !ok {
		return nil
	}
	return di.(Decompressor)
}
