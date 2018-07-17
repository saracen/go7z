package filters

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
)

type rangeDecoder struct {
	r      io.Reader
	nrange uint
	code   uint
}

func newRangeDecoder(r io.Reader) (*rangeDecoder, error) {
	rd := &rangeDecoder{
		r:      r,
		nrange: 0xffffffff,
	}

	for i := 0; i < 5; i++ {
		b, err := rd.ReadByte()
		if err != nil {
			return nil, err
		}

		rd.code = (rd.code << 8) | uint(b)
	}
	return rd, nil
}

func (rd *rangeDecoder) ReadByte() (byte, error) {
	var b [1]byte
	_, err := rd.r.Read(b[:])
	return b[0], err
}

const (
	numMoveBits          = 5
	numbitModelTotalBits = 11
	bitModelTotal        = uint(1) << numbitModelTotalBits

	numTopBits = 24
	topValue   = uint(1 << numTopBits)
)

type statusDecoder struct {
	prob uint
}

func newStatusDecoder() *statusDecoder {
	return &statusDecoder{prob: bitModelTotal / 2}
}

func (sd *statusDecoder) Decode(decoder *rangeDecoder) (uint, error) {
	var err error
	var b byte

	newBound := (decoder.nrange >> numbitModelTotalBits) * sd.prob
	if decoder.code < newBound {
		decoder.nrange = newBound
		sd.prob += (bitModelTotal - sd.prob) >> numMoveBits
		if decoder.nrange < topValue {
			if b, err = decoder.ReadByte(); err != nil {
				return 0, err
			}
			decoder.code = (decoder.code << 8) | uint(b)
			decoder.nrange <<= 8
		}
		return 0, nil
	}

	decoder.nrange -= newBound
	decoder.code -= newBound
	sd.prob -= sd.prob >> numMoveBits
	if decoder.nrange < topValue {
		if b, err = decoder.ReadByte(); err != nil {
			return 0, err
		}
		decoder.code = (decoder.code << 8) | uint(b)
		decoder.nrange <<= 8
	}
	return 1, nil
}

// BCJ2Decoder is a BCJ2 decoder.
type BCJ2Decoder struct {
	main *bufio.Reader
	call io.Reader
	jump io.Reader

	rangeDecoder  *rangeDecoder
	statusDecoder []*statusDecoder

	written  int64
	finished bool

	prevByte byte

	buf *bytes.Buffer
}

// NewBCJ2Decoder returns a new BCJ2 decoder.
func NewBCJ2Decoder(main, call, jump, rangedecoder io.Reader, limit int64) (*BCJ2Decoder, error) {
	rd, err := newRangeDecoder(rangedecoder)
	if err != nil {
		return nil, err
	}

	decoder := &BCJ2Decoder{
		main:          bufio.NewReader(main),
		call:          call,
		jump:          jump,
		rangeDecoder:  rd,
		statusDecoder: make([]*statusDecoder, 256+2),
		buf:           new(bytes.Buffer),
	}
	decoder.buf.Grow(1 << 16)

	for i := range decoder.statusDecoder {
		decoder.statusDecoder[i] = newStatusDecoder()
	}

	return decoder, nil
}

func (d *BCJ2Decoder) isJcc(b0, b1 byte) bool {
	return b0 == 0x0f && (b1&0xf0) == 0x80
}

func (d *BCJ2Decoder) isJ(b0, b1 byte) bool {
	return (b1&0xfe) == 0xe8 || d.isJcc(b0, b1)
}

func (d *BCJ2Decoder) index(b0, b1 byte) int {
	switch b1 {
	case 0xe8:
		return int(b0)
	case 0xe9:
		return 256
	}
	return 257
}

func (d *BCJ2Decoder) Read(p []byte) (int, error) {
	err := d.read()
	if err != nil && err != io.EOF {
		return 0, err
	}

	return d.buf.Read(p)
}

func (d *BCJ2Decoder) read() error {
	b := byte(0)

	var err error
	for i := 0; i < d.buf.Cap(); i++ {
		b, err = d.main.ReadByte()
		if err != nil {
			return err
		}

		d.written++
		if err = d.buf.WriteByte(b); err != nil {
			return err
		}

		if d.isJ(d.prevByte, b) {
			break
		}
		d.prevByte = b
	}

	if d.buf.Len() == d.buf.Cap() {
		return nil
	}

	bit, err := d.statusDecoder[d.index(d.prevByte, b)].Decode(d.rangeDecoder)
	if err != nil {
		return err
	}

	if bit == 1 {
		var r io.Reader
		if b == 0xe8 {
			r = d.call
		} else {
			r = d.jump
		}

		var dest uint32
		if err = binary.Read(r, binary.BigEndian, &dest); err != nil {
			return err
		}

		dest -= uint32(d.written + 4)
		if err = binary.Write(d.buf, binary.LittleEndian, dest); err != nil {
			return err
		}

		d.prevByte = byte(dest >> 24)
		d.written += 4
	} else {
		d.prevByte = b
	}

	return nil
}
