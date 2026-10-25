package media

import (
	"bytes"
	"testing"
)

// Разбор EXIF и картинок не должен падать ни на каком входе.
// go test -fuzz=FuzzExif ./internal/media
func FuzzExif(f *testing.F) {
	f.Add([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x10, 'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0})
	f.Add([]byte{0xFF, 0xD8})
	f.Fuzz(func(t *testing.T, b []byte) {
		if o := exifOrientation(b); o < 1 || o > 8 {
			t.Fatalf("orientation out of range: %d", o)
		}
	})
}

func FuzzProcess(f *testing.F) {
	f.Add(pngBomb())
	f.Add([]byte("\x89PNG\r\n\x1a\n"))
	f.Add([]byte("GIF89a"))
	f.Fuzz(func(t *testing.T, b []byte) {
		res, err := Process(bytes.NewReader(b))
		if err == nil && (res.Width > fullSide || res.Height > fullSide) {
			t.Fatalf("output too large: %dx%d", res.Width, res.Height)
		}
	})
}
