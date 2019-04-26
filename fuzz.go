// +build gofuzz

package go7z

import (
	"bytes"
	"io"
	"io/ioutil"
)

func Fuzz(data []byte) int {
	sz := new(Reader)
	if err := sz.init(bytes.NewReader(data), int64(len(data)), true); err != nil {
		return 0
	}

	for {
		_, err := sz.Next()
		if err == io.EOF {
			return 0
		}
		if err != nil {
			return 0
		}

		if _, err = io.Copy(ioutil.Discard, sz); err != nil {
			return 0
		}
	}

	return 1
}
