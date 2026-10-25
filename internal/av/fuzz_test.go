package av

import "testing"

func FuzzSniff(f *testing.F) {
	f.Add([]byte("#EXTM3U"))
	f.Add([]byte("ID3\x04"))
	f.Add([]byte("RIFF\x00\x00\x00\x00AVI "))
	f.Fuzz(func(t *testing.T, b []byte) {
		format, err := Sniff(b)
		if err == nil {
			switch format {
			case "mp3", "ogg", "flac", "wav", "mov", "matroska":
			default:
				t.Fatalf("unexpected format %q", format)
			}
		}
		if len(b) > 0 && b[0] == '#' && err == nil {
			t.Fatal("text playlist accepted")
		}
	})
}
