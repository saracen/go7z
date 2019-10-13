package go7z

import (
	"io"
	"io/ioutil"
	"testing"
)

func TestOpenReader(t *testing.T) {
	fs, closeall := fixtures.Fixtures([]string{"empty", "delta"}, nil)
	defer closeall.Close()

	for _, f := range fs {
		sz, err := OpenReader(f.Name)
		if err == io.EOF {
			if f.Archive == "empty" {
				continue
			}
			t.Fatal(err)
		}
		if err != nil {
			t.Fatal(err)
		}

		for {
			_, err := sz.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}

			if _, err = io.Copy(ioutil.Discard, sz); err != nil {
				t.Fatal(err)
			}
		}

		if err := sz.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReader(t *testing.T) {
	fs, closeall := fixtures.Fixtures([]string{"executable", "random"}, []string{"ppmd", "ppc", "arm"})
	defer closeall.Close()

	for _, f := range fs {
		sz, err := NewReader(f, f.Size)
		if err != nil {
			t.Fatalf("error reading %v: %v\n", f.Archive, err)
		}

		count := 0
		for {
			_, err := sz.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				panic(err)
			}

			if _, err = io.Copy(ioutil.Discard, sz); err != nil {
				t.Fatal(err)
			}
			count++
		}

		if count != f.Entries {
			t.Fatalf("expected %v entries, got %v\n", f.Entries, count)
		}
	}
}
