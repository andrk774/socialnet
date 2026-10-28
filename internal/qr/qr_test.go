package qr

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// Пример из разбора стандарта (1-M, «HELLO WORLD»): кодовые слова данных
// и ожидаемые слова коррекции.
func TestReedSolomon(t *testing.T) {
	data := []byte{32, 91, 11, 120, 209, 114, 220, 77, 67, 64, 236, 17, 236, 17, 236, 17}
	want := []byte{196, 35, 39, 119, 235, 215, 231, 226, 93, 23}
	if got := rsRemainder(data, rsGenerator(10)); !bytes.Equal(got, want) {
		t.Fatalf("ec: %v, want %v", got, want)
	}
}

// Строки формата из таблицы C.1 стандарта.
func TestFormatBits(t *testing.T) {
	for _, tc := range []struct {
		lvl  Level
		mask int
		want string
	}{{L, 0, "111011111000100"}, {M, 0, "101010000010010"}, {Q, 7, "010101111101101"}, {H, 3, "001100111010000"}} {
		c := newCode(1)
		c.drawFunctionPatterns()
		c.drawFormat(tc.lvl, tc.mask)
		var s strings.Builder // первая копия: (8,0..5),(8,7),(8,8),(7,8),(5..0,8) — биты 0..14
		for _, p := range [][2]int{{8, 0}, {8, 1}, {8, 2}, {8, 3}, {8, 4}, {8, 5}, {8, 7}, {8, 8}, {7, 8}, {5, 8}, {4, 8}, {3, 8}, {2, 8}, {1, 8}, {0, 8}} {
			if c.Dark(p[0], p[1]) {
				s.WriteByte('1')
			} else {
				s.WriteByte('0')
			}
		}
		got := reverse(s.String()) // бит 14 — первым
		if got != tc.want {
			t.Errorf("format %v/%d: %s, want %s", tc.lvl, tc.mask, got, tc.want)
		}
	}
}

func reverse(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

// Табличные проверки: каждая версия и уровень дают 4V+17 модулей, а сумма
// данных и коррекции совпадает с ёмкостью версии из стандарта.
func TestTables(t *testing.T) {
	total := []int{26, 44, 70, 100, 134, 172, 196, 242, 292, 346}
	for v := 1; v <= MaxVersion; v++ {
		for lvl := L; lvl <= H; lvl++ {
			s := specs[v-1][lvl]
			blocks := s.groups[0][0] + s.groups[1][0]
			if got := s.dataLen() + blocks*s.ec; got != total[v-1] {
				t.Errorf("v%d lvl %d: %d codewords, want %d", v, lvl, got, total[v-1])
			}
		}
	}
}

func TestEncode(t *testing.T) {
	url := "https://pohody.example/qr/e/12345"
	c, err := Encode([]byte(url), M)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 3 || c.Size != 29 {
		t.Errorf("version %d size %d", c.Version, c.Size)
	}
	// поисковые узоры в трёх углах: тёмная рамка 7×7 и светлый разделитель
	for _, p := range [][2]int{{0, 0}, {c.Size - 7, 0}, {0, c.Size - 7}} {
		if !c.Dark(p[0], p[1]) || !c.Dark(p[0]+6, p[1]+6) || c.Dark(p[0]+1, p[1]+1) || !c.Dark(p[0]+3, p[1]+3) {
			t.Errorf("finder at %v", p)
		}
	}
	for i := 8; i < c.Size-8; i++ { // синхрополосы
		if c.Dark(i, 6) != (i%2 == 0) || c.Dark(6, i) != (i%2 == 0) {
			t.Fatalf("timing at %d", i)
		}
	}
	if !c.Dark(8, c.Size-8) {
		t.Error("dark module")
	}
	// одинаковый вход — одинаковый код
	c2, _ := Encode([]byte(url), M)
	if c.SVG("#000000", "#ffffff") != c2.SVG("#000000", "#ffffff") {
		t.Error("not deterministic")
	}
	// версии растут с длиной; слишком длинное — ошибка
	if c, _ := Encode(bytes.Repeat([]byte("a"), 200), M); c == nil || c.Version != 10 {
		t.Errorf("200 bytes: %v", c)
	}
	if _, err := Encode(bytes.Repeat([]byte("a"), 300), M); !errors.Is(err, ErrTooLong) {
		t.Errorf("300 bytes: %v", err)
	}
}

func TestSVG(t *testing.T) {
	c, _ := Encode([]byte("x"), L)
	svg := c.SVG(`"/><script>`, "#0A4A3A")
	if strings.Contains(svg, "script") || !strings.Contains(svg, `fill="#000000"`) || !strings.Contains(svg, `fill="#0A4A3A"`) {
		t.Errorf("colors not sanitized: %s", svg[:200])
	}
	if !strings.HasPrefix(svg, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 29 29"`) {
		t.Errorf("viewBox: %s", svg[:80])
	}
}
