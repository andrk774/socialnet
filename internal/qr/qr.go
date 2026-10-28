// Package qr — кодировщик QR-кодов (ISO/IEC 18004) без внешних
// зависимостей: байтовый режим, версии 1–10, уровни коррекции L/M/Q/H.
// Этого хватает для ссылок до ~200 байт; длиннее — ошибка.
//
// Устройство по шагам: данные → кодовые слова с дополнением → блоки с
// кодом Рида — Соломона → чередование блоков → размещение змейкой →
// выбор маски с наименьшим штрафом → служебная информация (формат, версия).
package qr

import (
	"errors"
	"strconv"
	"strings"
)

type Level int

const (
	L Level = iota // ~7% восстановления
	M              // ~15%
	Q              // ~25%
	H              // ~30%
)

// formatBits — код уровня в служебной информации (стандарт: L=01, M=00, Q=11, H=10).
var formatBits = [4]int{1, 0, 3, 2}

// blockSpec: на блок — кодовых слов коррекции, затем группы (число блоков, данных в блоке).
type blockSpec struct {
	ec     int
	groups [2][2]int
}

// Таблица 9 стандарта для версий 1–10: [версия-1][уровень].
var specs = [10][4]blockSpec{
	{{7, [2][2]int{{1, 19}}}, {10, [2][2]int{{1, 16}}}, {13, [2][2]int{{1, 13}}}, {17, [2][2]int{{1, 9}}}},
	{{10, [2][2]int{{1, 34}}}, {16, [2][2]int{{1, 28}}}, {22, [2][2]int{{1, 22}}}, {28, [2][2]int{{1, 16}}}},
	{{15, [2][2]int{{1, 55}}}, {26, [2][2]int{{1, 44}}}, {18, [2][2]int{{2, 17}}}, {22, [2][2]int{{2, 13}}}},
	{{20, [2][2]int{{1, 80}}}, {18, [2][2]int{{2, 32}}}, {26, [2][2]int{{2, 24}}}, {16, [2][2]int{{4, 9}}}},
	{{26, [2][2]int{{1, 108}}}, {24, [2][2]int{{2, 43}}}, {18, [2][2]int{{2, 15}, {2, 16}}}, {22, [2][2]int{{2, 11}, {2, 12}}}},
	{{18, [2][2]int{{2, 68}}}, {16, [2][2]int{{4, 27}}}, {24, [2][2]int{{4, 19}}}, {28, [2][2]int{{4, 15}}}},
	{{20, [2][2]int{{2, 78}}}, {18, [2][2]int{{4, 31}}}, {18, [2][2]int{{2, 14}, {4, 15}}}, {26, [2][2]int{{4, 13}, {1, 14}}}},
	{{24, [2][2]int{{2, 97}}}, {22, [2][2]int{{2, 38}, {2, 39}}}, {22, [2][2]int{{4, 18}, {2, 19}}}, {26, [2][2]int{{4, 14}, {2, 15}}}},
	{{30, [2][2]int{{2, 116}}}, {22, [2][2]int{{3, 36}, {2, 37}}}, {20, [2][2]int{{4, 16}, {4, 17}}}, {24, [2][2]int{{4, 12}, {4, 13}}}},
	{{18, [2][2]int{{2, 68}, {2, 69}}}, {26, [2][2]int{{4, 43}, {1, 44}}}, {24, [2][2]int{{6, 19}, {2, 20}}}, {28, [2][2]int{{6, 15}, {2, 16}}}},
}

// Центры шаблонов выравнивания (таблица E.1) для версий 2–10.
var alignment = [11][]int{2: {6, 18}, 3: {6, 22}, 4: {6, 26}, 5: {6, 30}, 6: {6, 34},
	7: {6, 22, 38}, 8: {6, 24, 42}, 9: {6, 26, 46}, 10: {6, 28, 50}}

const MaxVersion = 10

var ErrTooLong = errors.New("qr: data too long (versions 1-10 only)")

func (s blockSpec) dataLen() int {
	n := 0
	for _, g := range s.groups {
		n += g[0] * g[1]
	}
	return n
}

// Code — готовый QR-код: Size×Size модулей, true — тёмный.
type Code struct {
	Version int
	Size    int
	Mask    int
	modules [][]bool
	fn      [][]bool // функциональные модули: данные и маска их не трогают
}

func (c *Code) Dark(x, y int) bool { return c.modules[y][x] }

// Encode кодирует байты в QR-код наименьшей подходящей версии.
func Encode(data []byte, lvl Level) (*Code, error) {
	if lvl < L || lvl > H {
		return nil, errors.New("qr: bad level")
	}
	version := 0
	for v := 1; v <= MaxVersion; v++ {
		countBits := 8
		if v >= 10 {
			countBits = 16
		}
		if 4+countBits+8*len(data) <= specs[v-1][lvl].dataLen()*8 {
			version = v
			break
		}
	}
	if version == 0 {
		return nil, ErrTooLong
	}
	spec := specs[version-1][lvl]

	// поток битов: режим «байты» 0100, длина, данные, терминатор, дополнение
	var bits bitBuffer
	bits.append(0b0100, 4)
	if version >= 10 {
		bits.append(len(data), 16)
	} else {
		bits.append(len(data), 8)
	}
	for _, b := range data {
		bits.append(int(b), 8)
	}
	capacity := spec.dataLen() * 8
	bits.append(0, min(4, capacity-len(bits)))
	bits.append(0, (8-len(bits)%8)%8)
	for pad := 0xEC; len(bits) < capacity; pad ^= 0xEC ^ 0x11 {
		bits.append(pad, 8)
	}
	codewords := bits.bytes()

	// блоки и коррекция Рида — Соломона, затем чередование
	gen := rsGenerator(spec.ec)
	var blocks, ecs [][]byte
	pos := 0
	for _, g := range spec.groups {
		for range g[0] {
			blk := codewords[pos : pos+g[1]]
			pos += g[1]
			blocks = append(blocks, blk)
			ecs = append(ecs, rsRemainder(blk, gen))
		}
	}
	var final []byte
	for i := 0; ; i++ {
		added := false
		for _, b := range blocks {
			if i < len(b) {
				final = append(final, b[i])
				added = true
			}
		}
		if !added {
			break
		}
	}
	for i := range spec.ec {
		for _, e := range ecs {
			final = append(final, e[i])
		}
	}

	c := newCode(version)
	c.drawFunctionPatterns()
	c.drawCodewords(final)
	best, bestPenalty := 0, -1
	for mask := range 8 {
		c.applyMask(mask)
		c.drawFormat(lvl, mask)
		if p := c.penalty(); bestPenalty < 0 || p < bestPenalty {
			best, bestPenalty = mask, p
		}
		c.applyMask(mask) // XOR — повторное применение снимает маску
	}
	c.applyMask(best)
	c.drawFormat(lvl, best)
	c.Mask = best
	return c, nil
}

// ---------- биты ----------

type bitBuffer []bool

func (b *bitBuffer) append(val, n int) {
	for i := n - 1; i >= 0; i-- {
		*b = append(*b, (val>>i)&1 == 1)
	}
}

func (b bitBuffer) bytes() []byte {
	out := make([]byte, len(b)/8)
	for i, bit := range b {
		if bit {
			out[i/8] |= 0x80 >> (i % 8)
		}
	}
	return out
}

// ---------- Рид — Соломон над GF(256), многочлен 0x11D ----------

func gfMul(x, y byte) byte {
	var z int
	for i := 7; i >= 0; i-- {
		z = (z << 1) ^ ((z >> 7) * 0x11D)
		z ^= ((int(y) >> i) & 1) * int(x)
	}
	return byte(z)
}

// rsGenerator — коэффициенты (x−α⁰)(x−α¹)…(x−α^(n−1)) без старшего.
func rsGenerator(n int) []byte {
	res := make([]byte, n)
	res[n-1] = 1
	root := byte(1)
	for range n {
		for j := range n {
			res[j] = gfMul(res[j], root)
			if j+1 < n {
				res[j] ^= res[j+1]
			}
		}
		root = gfMul(root, 2)
	}
	return res
}

func rsRemainder(data, gen []byte) []byte {
	res := make([]byte, len(gen))
	for _, b := range data {
		factor := b ^ res[0]
		copy(res, res[1:])
		res[len(res)-1] = 0
		for i, g := range gen {
			res[i] ^= gfMul(g, factor)
		}
	}
	return res
}

// ---------- матрица ----------

func newCode(version int) *Code {
	size := version*4 + 17
	c := &Code{Version: version, Size: size, modules: make([][]bool, size)}
	for i := range c.modules {
		c.modules[i] = make([]bool, size)
	}
	return c
}

func (c *Code) set(fn [][]bool, x, y int, dark bool) {
	c.modules[y][x] = dark
	fn[y][x] = true
}

func (c *Code) drawFunctionPatterns() {
	fn := make([][]bool, c.Size)
	for i := range fn {
		fn[i] = make([]bool, c.Size)
	}
	n := c.Size
	// синхрополосы
	for i := range n {
		c.set(fn, 6, i, i%2 == 0)
		c.set(fn, i, 6, i%2 == 0)
	}
	// поисковые узоры с разделителями
	for _, p := range [][2]int{{3, 3}, {n - 4, 3}, {3, n - 4}} {
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				x, y := p[0]+dx, p[1]+dy
				if x < 0 || y < 0 || x >= n || y >= n {
					continue
				}
				d := max(abs(dx), abs(dy))
				c.set(fn, x, y, d != 2 && d != 4)
			}
		}
	}
	// шаблоны выравнивания (кроме углов с поисковыми узорами)
	al := alignment[c.Version]
	for i, ax := range al {
		for j, ay := range al {
			if (i == 0 && j == 0) || (i == 0 && j == len(al)-1) || (i == len(al)-1 && j == 0) {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					c.set(fn, ax+dx, ay+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	// место под формат (заполняется позже) и тёмный модуль
	c.reserveFormat(fn)
	// информация о версии (7+)
	if c.Version >= 7 {
		rem := c.Version
		for range 12 {
			rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
		}
		bits := c.Version<<12 | rem
		for i := range 18 {
			bit := (bits>>i)&1 == 1
			a, b := n-11+i%3, i/3
			c.set(fn, a, b, bit)
			c.set(fn, b, a, bit)
		}
	}
	c.fn = fn
}

func (c *Code) reserveFormat(fn [][]bool) {
	n := c.Size
	for i := range 9 {
		fn[8][i], fn[i][8] = true, true
	}
	for i := range 8 {
		fn[8][n-1-i], fn[n-1-i][8] = true, true
	}
}

func (c *Code) drawFormat(lvl Level, mask int) {
	data := formatBits[lvl]<<3 | mask
	rem := data
	for range 10 {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	bits := (data<<10 | rem) ^ 0x5412
	bit := func(i int) bool { return (bits>>i)&1 == 1 }
	n := c.Size
	fn := c.fn
	for i := 0; i <= 5; i++ {
		c.set(fn, 8, i, bit(i))
	}
	c.set(fn, 8, 7, bit(6))
	c.set(fn, 8, 8, bit(7))
	c.set(fn, 7, 8, bit(8))
	for i := 9; i < 15; i++ {
		c.set(fn, 14-i, 8, bit(i))
	}
	for i := range 8 {
		c.set(fn, n-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		c.set(fn, 8, n-15+i, bit(i))
	}
	c.set(fn, 8, n-8, true) // всегда тёмный
}

func (c *Code) drawCodewords(data []byte) {
	n := c.Size
	i := 0
	for right := n - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := range n {
			for j := range 2 {
				x := right - j
				y := vert
				if (right+1)&2 == 0 {
					y = n - 1 - vert
				}
				if !c.fn[y][x] && i < len(data)*8 {
					c.modules[y][x] = (data[i>>3]>>(7-i&7))&1 == 1
					i++
				}
			}
		}
	}
}

func maskBit(mask, x, y int) bool {
	switch mask {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (x/3+y/2)%2 == 0
	case 5:
		return x*y%2+x*y%3 == 0
	case 6:
		return (x*y%2+x*y%3)%2 == 0
	default:
		return ((x+y)%2+x*y%3)%2 == 0
	}
}

func (c *Code) applyMask(mask int) {
	for y := range c.Size {
		for x := range c.Size {
			if !c.fn[y][x] && maskBit(mask, x, y) {
				c.modules[y][x] = !c.modules[y][x]
			}
		}
	}
}

// penalty — штраф по четырём правилам стандарта (раздел 7.8.3).
func (c *Code) penalty() int {
	n := c.Size
	p := 0
	at := func(x, y int, vertical bool) bool {
		if vertical {
			return c.modules[x][y]
		}
		return c.modules[y][x]
	}
	for _, vertical := range []bool{false, true} {
		for y := range n {
			run := 1
			for x := 1; x < n; x++ {
				if at(x, y, vertical) == at(x-1, y, vertical) {
					run++
					if run == 5 {
						p += 3
					} else if run > 5 {
						p++
					}
				} else {
					run = 1
				}
			}
			// 1:1:3:1:1 с четырьмя светлыми с одной стороны
			for x := 0; x+10 < n; x++ {
				var s strings.Builder
				for k := range 11 {
					if at(x+k, y, vertical) {
						s.WriteByte('1')
					} else {
						s.WriteByte('0')
					}
				}
				if v := s.String(); v == "10111010000" || v == "00001011101" {
					p += 40
				}
			}
		}
	}
	dark := 0
	for y := range n {
		for x := range n {
			if c.modules[y][x] {
				dark++
			}
			if x+1 < n && y+1 < n {
				v := c.modules[y][x]
				if c.modules[y][x+1] == v && c.modules[y+1][x] == v && c.modules[y+1][x+1] == v {
					p += 3
				}
			}
		}
	}
	total := n * n
	k := (abs(dark*20-total*10)+total-1)/total - 1
	return p + max(k, 0)*10
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// SVG — код векторной картинкой: поле 4 модуля, тёмные модули одним путём
// (горизонтальные отрезки склеены). Цвета — #RRGGBB.
func (c *Code) SVG(dark, light string) string {
	const quiet = 4
	full := c.Size + 2*quiet
	var path strings.Builder
	for y := range c.Size {
		for x := 0; x < c.Size; {
			if !c.modules[y][x] {
				x++
				continue
			}
			start := x
			for x < c.Size && c.modules[y][x] {
				x++
			}
			path.WriteString("M" + strconv.Itoa(start+quiet) + " " + strconv.Itoa(y+quiet) + "h" + strconv.Itoa(x-start) + "v1h-" + strconv.Itoa(x-start) + "z")
		}
	}
	vb := strconv.Itoa(full)
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` + vb + ` ` + vb + `" shape-rendering="crispEdges">` +
		`<rect width="` + vb + `" height="` + vb + `" fill="` + safeColor(light, "#ffffff") + `"/>` +
		`<path fill="` + safeColor(dark, "#000000") + `" d="` + path.String() + `"/></svg>`
}

// safeColor пропускает только #RGB и #RRGGBB — в SVG не попадёт ничего иного.
func safeColor(s, def string) string {
	if (len(s) == 4 || len(s) == 7) && s[0] == '#' {
		for _, r := range s[1:] {
			if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
				return def
			}
		}
		return s
	}
	return def
}
