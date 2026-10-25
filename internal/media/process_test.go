package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func testImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{uint8(x * 5), uint8(y * 5), 100, 255})
		}
	}
	return img
}

// jpegWithExif вставляет APP1/EXIF с Orientation и «секретом» вроде GPS.
func jpegWithExif(t *testing.T, w, h int, orientation uint16, secret string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(w, h), nil); err != nil {
		t.Fatal(err)
	}
	tiff := []byte("II*\x00\x08\x00\x00\x00")
	tiff = binary.LittleEndian.AppendUint16(tiff, 1)           // 1 запись
	tiff = binary.LittleEndian.AppendUint16(tiff, 0x0112)      // Orientation
	tiff = binary.LittleEndian.AppendUint16(tiff, 3)           // SHORT
	tiff = binary.LittleEndian.AppendUint32(tiff, 1)           // count
	tiff = binary.LittleEndian.AppendUint16(tiff, orientation) // value
	tiff = append(tiff, 0, 0, 0, 0, 0, 0)                      // padding + next IFD
	tiff = append(tiff, []byte(secret)...)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xFF, 0xE1}
	seg = binary.BigEndian.AppendUint16(seg, uint16(len(payload)+2))
	seg = append(seg, payload...)
	orig := buf.Bytes()
	out := append([]byte{}, orig[:2]...)
	out = append(out, seg...)
	return append(out, orig[2:]...)
}

func TestExifRotationAndStripping(t *testing.T) {
	data := jpegWithExif(t, 40, 20, 6, "GPS-SECRET-55.04N-60.10E")
	if exifOrientation(data) != 6 {
		t.Fatal("orientation not parsed")
	}
	res, err := Process(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if res.Width != 20 || res.Height != 40 {
		t.Fatalf("not rotated: %dx%d", res.Width, res.Height)
	}
	for _, b := range [][]byte{res.Full, res.Thumb} {
		if bytes.Contains(b, []byte("Exif")) || bytes.Contains(b, []byte("GPS-SECRET")) {
			t.Fatal("metadata survived")
		}
	}
}

func TestResizeAndThumb(t *testing.T) {
	var buf bytes.Buffer
	_ = png.Encode(&buf, testImage(3000, 1500))
	res, err := Process(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Mime != "image/png" || res.Width != 2560 || res.Height != 1280 {
		t.Fatalf("full: %s %dx%d", res.Mime, res.Width, res.Height)
	}
	th, _ := png.DecodeConfig(bytes.NewReader(res.Thumb))
	if th.Width != 320 || th.Height != 160 {
		t.Fatalf("thumb: %dx%d", th.Width, th.Height)
	}
}

// pngBomb — настоящий маленький PNG, у которого в заголовке размеры 100000x100000.
func pngBomb() []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, testImage(1, 1))
	b := buf.Bytes()
	binary.BigEndian.PutUint32(b[16:], 100000)
	binary.BigEndian.PutUint32(b[20:], 100000)
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	return b
}

func TestRejects(t *testing.T) {
	if _, err := Process(bytes.NewReader(pngBomb())); !errors.Is(err, ErrTooManyPixel) {
		t.Fatalf("bomb: %v", err)
	}
	if _, err := Process(bytes.NewReader([]byte("<?php system($_GET['c']); ?>"))); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("php: %v", err)
	}
	if _, err := Process(bytes.NewReader([]byte("<svg onload=alert(1)>"))); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("svg: %v", err)
	}
	// заголовок PNG, а дальше мусор
	if _, err := Process(bytes.NewReader(append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 100)...))); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := Process(bytes.NewReader(make([]byte, MaxBytes+10))); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("too large: %v", err)
	}
}

func TestPolyglotTrailerRemoved(t *testing.T) {
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, testImage(10, 10), nil)
	data := append(buf.Bytes(), []byte("PK\x03\x04 hidden zip / script <script>alert(1)</script>")...)
	res, err := Process(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(res.Full, []byte("<script>")) {
		t.Fatal("trailing payload survived")
	}
}

func TestDiskPathSafety(t *testing.T) {
	d, _ := NewDisk(t.TempDir())
	if err := d.Save("../../etc/passwd", "full", []byte("x")); err == nil {
		t.Fatal("path traversal accepted")
	}
	if _, _, err := d.Open("00000000-0000-0000-0000-000000000000", "../x"); err == nil {
		t.Fatal("bad size accepted")
	}
}
