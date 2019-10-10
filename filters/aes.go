package filters

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"io"
	"strings"
	"unicode/utf16"
)

var km keyManager

func init() {
	km.cache = make(map[string][]byte)
	km.hasher = sha256.New()
}

// AESDecrypter is an AES-256 decryptor.
type AESDecrypter struct {
	r    io.Reader
	rbuf bytes.Buffer
	cbc  cipher.BlockMode
	buf  [aes.BlockSize]byte
}

type keyManager struct {
	hasher hash.Hash
	cache  map[string][]byte
}

func (km *keyManager) Key(power int, salt []byte, password string) []byte {
	var cacheKey strings.Builder
	cacheKey.WriteString(password)
	cacheKey.Write(salt)
	cacheKey.WriteByte(byte(power))

	key, ok := km.cache[cacheKey.String()]
	if ok {
		return key
	}

	b := bytes.NewBuffer(nil)
	for _, p := range utf16.Encode([]rune(password)) {
		binary.Write(b, binary.LittleEndian, p)
	}

	if power == 0x3f {
		key = km.stretch(salt, b.Bytes())
	} else {
		key = km.sha256Stretch(power, salt, b.Bytes())
	}

	km.cache[cacheKey.String()] = key
	return key
}

func (km *keyManager) stretch(salt, password []byte) []byte {
	var key [aes.BlockSize]byte

	var pos int
	for pos = 0; pos < len(salt); pos++ {
		key[pos] = salt[pos]
	}
	for i := 0; i < len(password) && pos < len(key); i++ {
		key[pos] = password[i]
		pos++
	}
	for ; pos < len(key); pos++ {
		key[pos] = 0
	}
	return key[:]
}

func (km *keyManager) sha256Stretch(power int, salt, password []byte) []byte {
	var temp [8]byte
	for round := 0; round < 1<<power; round++ {
		km.hasher.Write(salt)
		km.hasher.Write(password)
		km.hasher.Write(temp[:])

		for i := 0; i < 8; i++ {
			temp[i]++
			if temp[i] != 0 {
				break
			}
		}
	}

	defer km.hasher.Reset()
	return km.hasher.Sum(nil)
}

// NewAESDecrypter returns a new AES-256 decryptor.
func NewAESDecrypter(r io.Reader, power int, salt, iv []byte, password string) (*AESDecrypter, error) {
	key := km.Key(power, salt, password)

	cb, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	var aesiv [aes.BlockSize]byte
	copy(aesiv[:], iv)

	return &AESDecrypter{
		r:   r,
		cbc: cipher.NewCBCDecrypter(cb, aesiv[:]),
	}, nil
}

func (d *AESDecrypter) Read(p []byte) (int, error) {
	for d.rbuf.Len() < len(p) {
		_, err := d.r.Read(d.buf[:])
		if err != nil {
			return 0, err
		}

		d.cbc.CryptBlocks(d.buf[:], d.buf[:])

		_, err = d.rbuf.Write(d.buf[:])
		if err != nil {
			return 0, err
		}
	}

	n, err := d.rbuf.Read(p)
	return n, err
}
