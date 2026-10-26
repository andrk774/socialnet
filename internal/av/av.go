// Package av: безопасная работа с аудио и видео через ffmpeg.
//
// ffmpeg разбирает файлы, присланные кем угодно, поэтому:
//   - контейнер определяется по сигнатуре байтов здесь, в Go, и передаётся
//     ffmpeg явно (-f). Текстовые форматы (плейлисты HLS, ffconcat, SDP)
//     отклоняются: через них ffmpeg можно заставить прочитать /etc/passwd
//     или сходить по сети (известная атака на сервисы обработки видео);
//   - разрешён только протокол file;
//   - AVI не принимается (атака через вложенные субтитры GAB2);
//   - каждый запуск ограничен по времени;
//   - на выходе всегда новый файл без метаданных.
package av

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"socialnet/internal/s3"
)

var (
	ErrUnsupported = errors.New("unsupported file format")
	ErrNoStream    = errors.New("file has no usable stream")
	ErrTooLong     = errors.New("duration is too long")
	ErrCorrupt     = errors.New("file is corrupt")
	ErrNotFound    = errors.New("file not found")
)

// Sniff определяет контейнер по первым байтам. Возвращает имя демультиплексора ffmpeg.
func Sniff(head []byte) (string, error) {
	h := head
	switch {
	case bytes.HasPrefix(h, []byte("ID3")):
		return "mp3", nil
	case len(h) >= 2 && h[0] == 0xFF && (h[1]&0xE0) == 0xE0 && (h[1]&0x06) != 0: // кадр MPEG audio
		return "mp3", nil
	case bytes.HasPrefix(h, []byte("OggS")):
		return "ogg", nil
	case bytes.HasPrefix(h, []byte("fLaC")):
		return "flac", nil
	case len(h) >= 12 && string(h[:4]) == "RIFF" && string(h[8:12]) == "WAVE":
		return "wav", nil
	// AVI сознательно не поддерживается: известная атака прятала HLS-плейлист
	// в субтитрах GAB2 внутри AVI, и ffmpeg читал по нему локальные файлы.
	case len(h) >= 12 && string(h[4:8]) == "ftyp": // mp4, m4a, mov, 3gp
		return "mov", nil
	case bytes.HasPrefix(h, []byte{0x1A, 0x45, 0xDF, 0xA3}): // mkv, webm
		return "matroska", nil
	}
	return "", ErrUnsupported
}

func SniffFile(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // путь создан нами: временный файл загрузки
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 16)
	n, _ := io.ReadFull(f, head)
	return Sniff(head[:n])
}

// baseArgs — общие безопасные параметры для ffmpeg и ffprobe.
func baseArgs() []string {
	return []string{"-hide_banner", "-v", "error", "-protocol_whitelist", "file"}
}

func run(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)  // #nosec G204 -- name — только ffmpeg/ffprobe, аргументы без оболочки
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")} // без лишних переменных окружения
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("%s timed out", name)
	}
	if err != nil {
		msg := stderr.String()
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(msg))
	}
	return out, nil
}

type Stream struct {
	CodecType   string `json:"codec_type"`
	CodecName   string `json:"codec_name"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Disposition struct {
		AttachedPic int `json:"attached_pic"`
	} `json:"disposition"`
	Tags map[string]string `json:"tags"`
}

type Probe struct {
	Format  string
	Streams []Stream
	Seconds float64
	Tags    map[string]string // ключи в нижнем регистре
}

func (p *Probe) Audio() *Stream {
	for i := range p.Streams {
		if p.Streams[i].CodecType == "audio" {
			return &p.Streams[i]
		}
	}
	return nil
}

// Video — первая настоящая видеодорожка (обложка альбома не считается).
func (p *Probe) Video() *Stream {
	for i := range p.Streams {
		if p.Streams[i].CodecType == "video" && p.Streams[i].Disposition.AttachedPic == 0 {
			return &p.Streams[i]
		}
	}
	return nil
}

// ProbeFile: формат определяется сигнатурой, ffprobe получает его явно.
func ProbeFile(ctx context.Context, path string) (*Probe, error) {
	format, err := SniffFile(path)
	if err != nil {
		return nil, err
	}
	args := append(baseArgs(), "-f", format, "-print_format", "json", "-show_format", "-show_streams", "file:"+path)
	out, err := run(ctx, 30*time.Second, "ffprobe", args...)
	if err != nil {
		return nil, ErrCorrupt
	}
	var raw struct {
		Streams []Stream `json:"streams"`
		Format  struct {
			Duration string            `json:"duration"`
			Tags     map[string]string `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, ErrCorrupt
	}
	p := &Probe{Format: format, Streams: raw.Streams, Tags: map[string]string{}}
	p.Seconds, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	for k, v := range raw.Format.Tags {
		p.Tags[strings.ToLower(k)] = v
	}
	return p, nil
}

// TranscodeAudio перекодирует в MP3 192 kbit/s без метаданных и обложек.
func TranscodeAudio(ctx context.Context, in, out, format string, maxSeconds int) error {
	args := append(baseArgs(), "-nostdin", "-y",
		"-f", format, "-i", "file:"+in,
		"-map", "0:a:0", "-vn", "-sn", "-dn", "-map_metadata", "-1", "-map_chapters", "-1",
		"-c:a", "libmp3lame", "-b:a", "192k", "-ar", "44100",
		"-t", strconv.Itoa(maxSeconds),
		"-f", "mp3", "file:"+out)
	_, err := run(ctx, 5*time.Minute, "ffmpeg", args...)
	return err
}

// TranscodeVoice — голосовое: MP3 64 kbit/s моно, без метаданных.
func TranscodeVoice(ctx context.Context, in, out, format string, maxSeconds int) error {
	args := append(baseArgs(), "-nostdin", "-y",
		"-f", format, "-i", "file:"+in,
		"-map", "0:a:0", "-vn", "-sn", "-dn", "-map_metadata", "-1",
		"-c:a", "libmp3lame", "-b:a", "64k", "-ac", "1", "-ar", "44100",
		"-t", strconv.Itoa(maxSeconds),
		"-f", "mp3", "file:"+out)
	_, err := run(ctx, 2*time.Minute, "ffmpeg", args...)
	return err
}

// Rendition — одно качество HLS.
type Rendition struct {
	Name     string // 360p
	Height   int
	VBitrate string // 800k
	ABitrate string
}

var Renditions = []Rendition{
	{"360p", 360, "800k", "96k"},
	{"720p", 720, "2800k", "128k"},
}

// TranscodeHLS режет видео на сегменты HLS в каталог dir. Качество выше
// исходного не делается. Возвращает список созданных качеств.
func TranscodeHLS(ctx context.Context, in, format, dir string, srcHeight int, maxSeconds int, hasAudio bool) ([]string, error) {
	var made []string
	for _, r := range Renditions {
		if r.Height > srcHeight && len(made) > 0 {
			break // не растягиваем маленькое видео
		}
		args := append(baseArgs(), "-nostdin", "-y",
			"-f", format, "-i", "file:"+in,
			"-map", "0:v:0")
		if hasAudio {
			args = append(args, "-map", "0:a:0?", "-c:a", "aac", "-b:a", r.ABitrate, "-ac", "2")
		}
		args = append(args,
			"-map_metadata", "-1", "-sn", "-dn",
			"-t", strconv.Itoa(maxSeconds),
			// чётная ширина, высота не больше нужной
			"-vf", fmt.Sprintf("scale=-2:'min(%d,ih)'", r.Height),
			"-c:v", "libx264", "-preset", "veryfast", "-profile:v", "main", "-pix_fmt", "yuv420p",
			"-b:v", r.VBitrate, "-maxrate", r.VBitrate, "-bufsize", r.VBitrate,
			"-g", "48", "-keyint_min", "48", "-sc_threshold", "0",
			"-f", "hls", "-hls_time", "4", "-hls_playlist_type", "vod",
			"-hls_segment_filename", filepath.Join(dir, r.Name+"_%04d.ts"),
			filepath.Join(dir, r.Name+".m3u8"))
		if _, err := run(ctx, 30*time.Minute, "ffmpeg", args...); err != nil {
			return nil, err
		}
		made = append(made, r.Name)
	}
	if len(made) == 0 {
		return nil, ErrNoStream
	}
	// главный плейлист: плеер сам выберет качество под скорость сети
	var m strings.Builder
	m.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n")
	for _, name := range made {
		for _, r := range Renditions {
			if r.Name == name {
				bw := (bitrate(r.VBitrate) + bitrate(r.ABitrate)) * 1000
				fmt.Fprintf(&m, "#EXT-X-STREAM-INF:BANDWIDTH=%d,NAME=\"%s\"\n%s.m3u8\n", bw, r.Name, r.Name)
			}
		}
	}
	return made, os.WriteFile(filepath.Join(dir, "master.m3u8"), []byte(m.String()), 0o600)
}

// RemuxMP4 склеивает сегменты готового качества в один MP4 без перекодирования
// (-c copy: секунды работы). Нужен браузерам без встроенного HLS (Chrome и
// Firefox на компьютере). faststart — индекс в начале файла, видео начинает
// играть до полной загрузки. Вход — наш собственный плейлист, не файл
// пользователя, поэтому разбирать HLS здесь безопасно.
func RemuxMP4(ctx context.Context, dir, rendition string) error {
	if !fileRe.MatchString(rendition + ".m3u8") {
		return ErrNotFound
	}
	args := append(baseArgs(), "-nostdin", "-y",
		"-f", "hls", "-i", "file:"+filepath.Join(dir, rendition+".m3u8"),
		"-map", "0", "-c", "copy", "-bsf:a", "aac_adtstoasc", "-movflags", "+faststart",
		"-f", "mp4", filepath.Join(dir, rendition+".mp4"))
	_, err := run(ctx, 10*time.Minute, "ffmpeg", args...)
	return err
}

// Fetch кладёт файл обработанного медиа в dst: с диска или из S3.
func (s *Store) Fetch(ctx context.Context, kind, id, name, dst string) error {
	if s.Remote == nil {
		src, err := s.Path(kind, id, name)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(src) //nolint:gosec // путь из белого списка
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o600) // #nosec G703 -- dst строит Store.Path из проверенных kind, UUID и имени по белому списку
	}
	key, err := s.Key(kind, id, name)
	if err != nil {
		return err
	}
	obj, err := s.Remote.Get(ctx, key, "")
	if err != nil {
		return err
	}
	defer func() { _ = obj.Body.Close() }()
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // dst — наш временный файл
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, obj.Body); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// PutFile выкладывает один файл (с диска dir) рядом с уже обработанными.
func (s *Store) PutFile(ctx context.Context, kind, id, name, srcDir string) error {
	src := filepath.Join(srcDir, name)
	if s.Remote == nil {
		dst, err := s.Path(kind, id, name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return err
		}
		return os.Rename(src, dst)
	}
	key, err := s.Key(kind, id, name)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(src) //nolint:gosec // наш временный файл
	if err != nil {
		return err
	}
	return s.Remote.Put(ctx, key, data, contentTypes[filepath.Ext(name)])
}

func bitrate(s string) int {
	n, _ := strconv.Atoi(strings.TrimSuffix(s, "k"))
	return n
}

// Poster — кадр на 1-й секунде (или первый, если видео короче).
func Poster(ctx context.Context, in, format, out string, seconds float64) error {
	at := "1"
	if seconds < 2 {
		at = "0"
	}
	args := append(baseArgs(), "-nostdin", "-y", "-ss", at,
		"-f", format, "-i", "file:"+in,
		"-frames:v", "1", "-vf", "scale=-2:'min(720,ih)'", "-map_metadata", "-1",
		"-f", "image2", "file:"+out)
	_, err := run(ctx, time.Minute, "ffmpeg", args...)
	return err
}

// ---------- хранилище ----------

var (
	idRe   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	fileRe = regexp.MustCompile(`^(file\.bin|audio\.mp3|voice\.mp3|master\.m3u8|poster\.jpg|(360p|720p)\.(m3u8|mp4)|(360p|720p)_[0-9]{4}\.ts)$`)
)

// Store хранит обработанные файлы: <root>/<kind>/<id[:2]>/<id>/<имя>.
// Путь собирается только из проверенного UUID и имени из белого списка.
// С Remote (S3) локальный диск — только место обработки: Commit выгружает
// готовые файлы в хранилище с ключами <kind>/<id>/<имя> и чистит диск.
type Store struct {
	root   string
	Remote *s3.Client
}

func NewStore(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func (s *Store) Dir(kind, id string) (string, error) {
	if (kind != "tracks" && kind != "videos" && kind != "voices" && kind != "docs") || !idRe.MatchString(id) {
		return "", ErrNotFound
	}
	return filepath.Join(s.root, kind, id[:2], id), nil
}

func (s *Store) Path(kind, id, name string) (string, error) {
	dir, err := s.Dir(kind, id)
	if err != nil || !fileRe.MatchString(name) {
		return "", ErrNotFound
	}
	return filepath.Join(dir, name), nil
}

func (s *Store) Delete(kind, id string) error {
	dir, err := s.Dir(kind, id)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if s.Remote == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	keys, err := s.Remote.List(ctx, kind+"/"+id+"/")
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := s.Remote.Delete(ctx, k); err != nil {
			return err
		}
	}
	return nil
}

// Key — ключ объекта в S3 (имя проверено белым списком).
func (s *Store) Key(kind, id, name string) (string, error) {
	if _, err := s.Path(kind, id, name); err != nil {
		return "", err
	}
	return kind + "/" + id + "/" + name, nil
}

var contentTypes = map[string]string{".mp3": "audio/mpeg", ".m3u8": "application/vnd.apple.mpegurl",
	".ts": "video/mp2t", ".jpg": "image/jpeg", ".mp4": "video/mp4", ".bin": "application/octet-stream"}

// Commit выгружает обработанные файлы в S3 (если оно настроено).
func (s *Store) Commit(ctx context.Context, kind, id string) error {
	if s.Remote == nil {
		return nil
	}
	dir, err := s.Dir(kind, id)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		key, err := s.Key(kind, id, e.Name())
		if err != nil || e.IsDir() {
			continue // посторонние файлы ffmpeg не выгружаем
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // имя из белого списка
		if err != nil {
			return err
		}
		if err := s.Remote.Put(ctx, key, data, contentTypes[filepath.Ext(e.Name())]); err != nil {
			return err
		}
	}
	return os.RemoveAll(dir)
}

// SourcePath — исходник видео, ждущий обработки в очереди.
func (s *Store) SourcePath(id string) (string, error) {
	if !idRe.MatchString(id) {
		return "", ErrNotFound
	}
	dir := filepath.Join(s.root, "sources")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	return filepath.Join(dir, id), nil
}

// TempDir — место для исходников, пока их обрабатывают.
func (s *Store) TempDir() (string, error) {
	dir := filepath.Join(s.root, "tmp")
	return dir, os.MkdirAll(dir, 0o750)
}

// NewID — случайный UUID v4.
func NewID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func Available() bool {
	_, e1 := exec.LookPath("ffmpeg")
	_, e2 := exec.LookPath("ffprobe")
	return e1 == nil && e2 == nil
}
