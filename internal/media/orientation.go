package media

import (
	"encoding/binary"
	"image"
)

// exifOrientation достаёт тег Orientation (0x0112) из EXIF в JPEG.
// Телефоны часто хранят фото «боком» и пишут в EXIF, как его повернуть.
// Раз мы удаляем EXIF, поворот нужно применить к самим пикселям.
// Возвращает 1 (без поворота), если тега нет или файл странный.
func exifOrientation(jpg []byte) int {
	if len(jpg) < 4 || jpg[0] != 0xFF || jpg[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(jpg) {
		if jpg[i] != 0xFF {
			return 1
		}
		marker := jpg[i+1]
		if marker == 0xDA || marker == 0xD9 { // начались данные изображения
			return 1
		}
		size := int(binary.BigEndian.Uint16(jpg[i+2:]))
		if size < 2 || i+2+size > len(jpg) {
			return 1
		}
		seg := jpg[i+4 : i+2+size]
		if marker == 0xE1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return parseTIFFOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 1
}

func parseTIFFOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	ifd := int(bo.Uint32(t[4:]))
	if ifd < 8 || ifd+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[ifd:]))
	for k := 0; k < n; k++ {
		e := ifd + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 {
			v := int(bo.Uint16(t[e+8:]))
			if v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// applyOrientation поворачивает/отражает изображение по значению EXIF 1..8.
func applyOrientation(img image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return img
	}
	src := toNRGBA(img)
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dw, dh := w, h
	if o >= 5 { // 5..8 меняют местами ширину и высоту
		dw, dh = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			copy(dst.Pix[dy*dst.Stride+dx*4:dy*dst.Stride+dx*4+4], src.Pix[y*src.Stride+x*4:y*src.Stride+x*4+4])
		}
	}
	return dst
}
