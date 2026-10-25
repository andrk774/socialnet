package redisx

import (
	"bufio"
	"bytes"
	"testing"
)

// Ответ сервера (или того, кто притворился им) не должен ронять процесс
// или заставлять выделять гигабайты памяти.
func FuzzReadReply(f *testing.F) {
	for _, s := range []string{"+OK\r\n", "-ERR x\r\n", ":1\r\n", "$3\r\nabc\r\n", "*2\r\n$1\r\na\r\n:2\r\n", "$-1\r\n", "*-1\r\n", "$999999999999\r\n"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		r := bufio.NewReader(bytes.NewReader(data))
		for i := 0; i < 10; i++ {
			if _, err := readReply(r); err != nil {
				return
			}
		}
	})
}
