// Package media: безопасная обработка загруженных изображений.
//
// Каждая картинка декодируется и кодируется заново. Это:
//   - удаляет EXIF и прочие метаданные (GPS-координаты, модель телефона);
//   - выбрасывает всё, что было «приклеено» к файлу (polyglot-файлы вроде GIFAR);
//   - гарантирует, что мы отдаём именно картинку указанного типа.
package media

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
)

var (
	ErrTooLarge     = errors.New("file too large")
	ErrUnsupported  = errors.New("unsupported file type")
	ErrTooManyPixel = errors.New("image dimensions too large")
	ErrCorrupt      = errors.New("corrupt image")
)

const (
	MaxBytes  = 10 << 20 // 10 MiB на файл
	maxSide   = 8000     // защита от «декомпрессионных бомб»
	maxPixels = 40_000_000
	fullSide  = 2560 // оригинал ужимаем до этого размера
	thumbSide = 320
)

type Result struct {
	Mime   string
	Width  int
	Height int
	Full   []byte
	Thumb  []byte
}

// Process читает не больше MaxBytes, проверяет и перекодирует изображение.
func Process(r io.Reader) (*Result, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxBytes {
		return nil, ErrTooLarge
	}

	// Тип определяем по содержимому, а не по имени файла или Content-Type клиента.
	sniffed := http.DetectContentType(data)
	switch sniffed {
	case "image/jpeg", "image/png", "image/gif":
	default:
		return nil, ErrUnsupported
	}

	// Сначала только заголовок: огромные размеры отсекаем до выделения памяти.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, ErrCorrupt
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxSide || cfg.Height > maxSide ||
		cfg.Width*cfg.Height > maxPixels {
		return nil, ErrTooManyPixel
	}

	var img image.Image
	switch sniffed {
	case "image/jpeg":
		img, err = jpeg.Decode(bytes.NewReader(data))
	case "image/png":
		img, err = png.Decode(bytes.NewReader(data))
	case "image/gif":
		img, err = gif.Decode(bytes.NewReader(data)) // первый кадр
	}
	if err != nil {
		return nil, ErrCorrupt
	}

	if sniffed == "image/jpeg" {
		img = applyOrientation(img, exifOrientation(data))
	}

	full := resizeToFit(img, fullSide)
	thumb := resizeToFit(img, thumbSide)

	res := &Result{Width: full.Bounds().Dx(), Height: full.Bounds().Dy()}
	if sniffed == "image/jpeg" {
		res.Mime = "image/jpeg"
		res.Full, err = encodeJPEG(full)
		if err == nil {
			res.Thumb, err = encodeJPEG(thumb)
		}
	} else {
		res.Mime = "image/png"
		res.Full, err = encodePNG(full)
		if err == nil {
			res.Thumb, err = encodePNG(thumb)
		}
	}
	return res, err
}

func encodeJPEG(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 85})
	return b.Bytes(), err
}

func encodePNG(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&b, img)
	return b.Bytes(), err
}

func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}

// resizeToFit уменьшает изображение так, чтобы большая сторона была <= side.
// Метод усреднения по площади: для уменьшения даёт чистый результат без «лесенки».
func resizeToFit(img image.Image, side int) *image.NRGBA {
	src := toNRGBA(img)
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	if sw <= side && sh <= side {
		return src
	}
	dw, dh := side, side
	if sw >= sh {
		dh = max(1, sh*side/sw)
	} else {
		dw = max(1, sw*side/sh)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		y0, y1 := y*sh/dh, max((y+1)*sh/dh, y*sh/dh+1)
		for x := 0; x < dw; x++ {
			x0, x1 := x*sw/dw, max((x+1)*sw/dw, x*sw/dw+1)
			var r, g, b, a, n uint64
			for yy := y0; yy < y1; yy++ {
				off := yy*src.Stride + x0*4
				for xx := x0; xx < x1; xx++ {
					pa := uint64(src.Pix[off+3])
					r += uint64(src.Pix[off]) * pa
					g += uint64(src.Pix[off+1]) * pa
					b += uint64(src.Pix[off+2]) * pa
					a += pa
					n++
					off += 4
				}
			}
			o := y*dst.Stride + x*4
			if a > 0 {
				dst.Pix[o], dst.Pix[o+1], dst.Pix[o+2] = uint8(r/a), uint8(g/a), uint8(b/a)
			}
			dst.Pix[o+3] = uint8(a / n)
		}
	}
	return dst
}
