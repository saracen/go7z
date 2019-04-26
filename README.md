# go7z

A native Go 7z archive reader.

Features:
- Development in early stages.
- Very little tests.
- Medium probability of crashes.
- Medium probability of using all memory.
- Decompresses:
  - [LZMA](https://github.com/ulikunitz/xz)
  - [LZMA2](https://github.com/ulikunitz/xz)
  - Delta
  - BCJ2
  - bzip2
  - deflate

## Usage
Extracting an archive:

```
package main

import (
	"io"
	"os"

	"github.com/saracen/go7z"
)

func main() {
	sz, err := go7z.OpenReader("hello.7z")
	if err != nil {
		panic(err)
	}
	defer sz.Close()

	for {
		hdr, err := sz.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			panic(err)
		}

		// If empty stream (no contents) and isn't specifically an empty file...
		// then it's a directory.
		if hdr.IsEmptyStream && !hdr.IsEmptyFile {
			if err := os.MkdirAll(hdr.Name, os.ModePerm); err != nil {
				panic(err)
			}
			continue
		}

		// Create file
		f, err := os.Create(hdr.Name)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		if _, err := io.Copy(f, sz); err != nil {
			panic(err)
		}
	}
}
```
