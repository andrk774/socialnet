// Package backup: резервные копии базы — pg_dump, шифрование, хранение
// (диск и/или S3), ротация и обязательная проверка восстановлением.
package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
)

// Формат шифрованного файла: "SNBK1" ‖ nonce-префикс (8 байт) ‖ блоки.
// Блок: длина (4 байта) ‖ AES-256-GCM(до 64 КиБ). Nonce блока = префикс ‖ номер,
// флаг последнего блока входит в AAD — обрезанный файл не расшифруется.
const (
	magic     = "SNBK1"
	chunkSize = 64 << 10
)

var ErrCorrupt = errors.New("backup: file is corrupt or the key is wrong")

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("backup: key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func nonce(prefix []byte, n uint32) []byte {
	out := make([]byte, 12)
	copy(out, prefix)
	binary.BigEndian.PutUint32(out[8:], n)
	return out
}

func aad(last bool) []byte {
	if last {
		return []byte{1}
	}
	return []byte{0}
}

// Encrypt шифрует поток src в dst.
func Encrypt(dst io.Writer, src io.Reader, key []byte) error {
	gcm, err := newGCM(key)
	if err != nil {
		return err
	}
	prefix := make([]byte, 8)
	if _, err := rand.Read(prefix); err != nil {
		return err
	}
	if _, err := dst.Write(append([]byte(magic), prefix...)); err != nil {
		return err
	}
	buf, next := make([]byte, chunkSize), make([]byte, chunkSize)
	n, err := io.ReadFull(src, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return err
	}
	for i := uint32(0); ; i++ {
		// читаем следующий блок заранее, чтобы знать, последний ли текущий
		m, err2 := io.ReadFull(src, next)
		if err2 != nil && !errors.Is(err2, io.ErrUnexpectedEOF) && !errors.Is(err2, io.EOF) {
			return err2
		}
		last := m == 0
		sealed := gcm.Seal(nil, nonce(prefix, i), buf[:n], aad(last))
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(sealed))) //nolint:gosec // не больше chunkSize+16
		if _, err := dst.Write(l[:]); err != nil {
			return err
		}
		if _, err := dst.Write(sealed); err != nil {
			return err
		}
		if last {
			return nil
		}
		if i == ^uint32(0) {
			return errors.New("backup: too large")
		}
		buf, next, n = next, buf, m
	}
}

// Decrypt проверяет и расшифровывает поток.
func Decrypt(dst io.Writer, src io.Reader, key []byte) error {
	gcm, err := newGCM(key)
	if err != nil {
		return err
	}
	head := make([]byte, len(magic)+8)
	if _, err := io.ReadFull(src, head); err != nil || string(head[:len(magic)]) != magic {
		return ErrCorrupt
	}
	prefix := head[len(magic):]
	for i := uint32(0); ; i++ {
		var l [4]byte
		_, err := io.ReadFull(src, l[:])
		if errors.Is(err, io.EOF) {
			return ErrCorrupt // файл кончился без блока с флагом «последний»
		}
		if err != nil {
			return ErrCorrupt
		}
		size := binary.BigEndian.Uint32(l[:])
		if size > chunkSize+uint32(gcm.Overhead()) {
			return ErrCorrupt
		}
		sealed := make([]byte, size)
		if _, err := io.ReadFull(src, sealed); err != nil {
			return ErrCorrupt
		}
		if plain, err := gcm.Open(nil, nonce(prefix, i), sealed, aad(true)); err == nil {
			if _, err := dst.Write(plain); err != nil {
				return err
			}
			if n, _ := src.Read(make([]byte, 1)); n != 0 {
				return ErrCorrupt // мусор после последнего блока
			}
			return nil
		}
		plain, err := gcm.Open(nil, nonce(prefix, i), sealed, aad(false))
		if err != nil {
			return ErrCorrupt
		}
		if _, err := dst.Write(plain); err != nil {
			return err
		}
	}
}
