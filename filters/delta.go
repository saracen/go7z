package filters

import "io"

const deltaStateSize = 256

// DeltaDecoder is a Delta decoder.
type DeltaDecoder struct {
	state [deltaStateSize]byte
	r     io.Reader
	delta uint
}

// NewDeltaDecoder returns a new Delta decoder.
func NewDeltaDecoder(r io.Reader, delta uint, limit int64) (*DeltaDecoder, error) {
	return &DeltaDecoder{r: r, delta: delta}, nil
}

func (d *DeltaDecoder) Read(p []byte) (int, error) {
	n, err := d.r.Read(p)
	if err != nil {
		return n, err
	}

	var buf [deltaStateSize]byte
	copy(buf[:], d.state[:d.delta])

	var i, j uint
	for i = 0; i < uint(n); {
		for j = 0; j < d.delta && i < uint(n); i++ {
			p[i] = buf[j] + p[i]
			buf[j] = p[i]
			j++
		}
	}

	if j == d.delta {
		j = 0
	}

	copy(d.state[:], buf[j:d.delta])
	copy(d.state[d.delta-j:], buf[:j])

	return n, err
}
