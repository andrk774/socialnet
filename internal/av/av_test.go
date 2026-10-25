package av

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func need(t *testing.T) {
	if !Available() {
		t.Skip("ffmpeg not installed")
	}
}

// gen создаёт тестовый файл генераторами ffmpeg (тон, тестовая картинка).
func gen(t *testing.T, name string, args ...string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), name)
	full := append([]string{"-hide_banner", "-v", "error", "-y"}, args...)
	full = append(full, out)
	if b, err := exec.Command("ffmpeg", full...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg gen: %v %s", err, b)
	}
	return out
}

func TestSniff(t *testing.T) {
	ok := map[string]string{
		"ID3\x04\x00":                  "mp3",
		"OggS\x00\x02":                 "ogg",
		"fLaC\x00\x00":                 "flac",
		"RIFF\x00\x00\x00\x00WAVEfmt ": "wav",
		"\x00\x00\x00\x20ftypisom":     "mov",
		"\x1a\x45\xdf\xa3\x01":         "matroska",
	}
	for head, want := range ok {
		if got, err := Sniff([]byte(head)); err != nil || got != want {
			t.Errorf("%q: %s %v", head, got, err)
		}
	}
	bad := []string{
		"#EXTM3U\n#EXTINF:10,\nfile:///etc/passwd", // HLS-плейлист: чтение локальных файлов
		"ffconcat version 1.0\nfile /etc/passwd",   // concat-демультиплексор
		"RIFF\x00\x00\x00\x00AVI LIST",             // AVI с GAB2
		"v=0\no=- 0 0 IN IP4 127.0.0.1",            // SDP: сетевые потоки
		"<?php system($_GET['c']);",
		"",
	}
	for _, head := range bad {
		if _, err := Sniff([]byte(head)); err == nil {
			t.Errorf("accepted dangerous %q", head)
		}
	}
}

func TestAudioPipeline(t *testing.T) {
	need(t)
	ctx := context.Background()
	src := gen(t, "in.mp3", "-f", "lavfi", "-i", "sine=frequency=440:duration=3",
		"-metadata", "title=Тестовый трек", "-metadata", "artist=Миасс Бэнд",
		"-metadata", "comment=GPS 55.04N", "-c:a", "libmp3lame")
	p, err := ProbeFile(ctx, src)
	if err != nil || p.Audio() == nil || p.Seconds < 2.5 || p.Tags["title"] != "Тестовый трек" {
		t.Fatalf("probe: %+v %v", p, err)
	}
	out := filepath.Join(t.TempDir(), "out.mp3")
	if err := TranscodeAudio(ctx, src, out, p.Format, 1200); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	if strings.Contains(string(b), "GPS") || strings.Contains(string(b), "Миасс") {
		t.Fatal("metadata survived transcoding")
	}
	p2, err := ProbeFile(ctx, out)
	if err != nil || p2.Audio().CodecName != "mp3" {
		t.Fatalf("output: %+v %v", p2, err)
	}
}

func TestHLSPipeline(t *testing.T) {
	need(t)
	ctx := context.Background()
	src := gen(t, "in.mp4", "-f", "lavfi", "-i", "testsrc=size=1280x720:rate=24:duration=3",
		"-f", "lavfi", "-i", "sine=duration=3", "-shortest", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac")
	p, err := ProbeFile(ctx, src)
	if err != nil || p.Video() == nil || p.Video().Height != 720 {
		t.Fatalf("probe: %+v %v", p, err)
	}
	dir := t.TempDir()
	made, err := TranscodeHLS(ctx, src, p.Format, dir, p.Video().Height, 600, p.Audio() != nil)
	if err != nil || len(made) != 2 {
		t.Fatalf("hls: %v %v", made, err)
	}
	master, _ := os.ReadFile(filepath.Join(dir, "master.m3u8"))
	if !strings.Contains(string(master), "360p.m3u8") || !strings.Contains(string(master), "720p.m3u8") {
		t.Fatalf("master: %s", master)
	}
	if err := Poster(ctx, src, p.Format, filepath.Join(dir, "poster.jpg"), p.Seconds); err != nil {
		t.Fatal(err)
	}
	// маленькое видео не растягивается до 720p
	small := gen(t, "small.mp4", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=24:duration=2", "-c:v", "libx264", "-preset", "ultrafast")
	ps, _ := ProbeFile(ctx, small)
	made, err = TranscodeHLS(ctx, small, ps.Format, t.TempDir(), ps.Video().Height, 600, false)
	if err != nil || len(made) != 1 || made[0] != "360p" {
		t.Fatalf("small hls: %v %v", made, err)
	}
}

func TestStorePaths(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	id := NewID()
	if _, err := s.Path("tracks", id, "audio.mp3"); err != nil {
		t.Fatal(err)
	}
	for _, c := range [][3]string{
		{"tracks", "../../etc", "audio.mp3"},
		{"tracks", id, "../../../etc/passwd"},
		{"tracks", id, "720p_1.ts"},
		{"secrets", id, "audio.mp3"},
	} {
		if _, err := s.Path(c[0], c[1], c[2]); err == nil {
			t.Errorf("accepted %v", c)
		}
	}
}
