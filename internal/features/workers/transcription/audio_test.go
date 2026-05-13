package transcription

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// requireFFmpeg skips the test when ffmpeg is missing. CI installs it via
// the Dockerfile change in Task 15; local devs can apt-install or brew.
func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH — skipping audio test")
	}
}

// makeSilentMP3 produces a `seconds`-long silent MP3 fixture using
// ffmpeg's anullsrc filter. We can't ship binary fixtures cleanly, so the
// tests build their own.
func makeSilentMP3(t *testing.T, dir string, seconds int) string {
	t.Helper()
	out := filepath.Join(dir, "silent.mp3")
	cmd := exec.Command("ffmpeg",
		"-y", "-loglevel", "error",
		"-f", "lavfi", "-i", "anullsrc=r=16000:cl=mono",
		"-t", fmt.Sprintf("%d", seconds),
		"-acodec", "libmp3lame", out,
	)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg fixture: %v (%s)", err, string(b))
	}
	return out
}

func TestExtractAudioMP3_HappyPath(t *testing.T) {
	requireFFmpeg(t)
	tmp := t.TempDir()
	src := makeSilentMP3(t, tmp, 1)
	dst := filepath.Join(tmp, "out.mp3")

	out, err := extractAudioMP3(context.Background(), src, dst)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("expected non-empty MP3, got 0 bytes")
	}
}

func TestExtractAudioMP3_BadInputFails(t *testing.T) {
	requireFFmpeg(t)
	tmp := t.TempDir()
	_, err := extractAudioMP3(context.Background(),
		filepath.Join(tmp, "does-not-exist.mp4"),
		filepath.Join(tmp, "out.mp3"))
	if err == nil {
		t.Fatal("expected error for missing input, got nil")
	}
}

func TestSplitAudioByDuration_ProducesMultipleParts(t *testing.T) {
	requireFFmpeg(t)
	tmp := t.TempDir()
	// 30s silent MP3, split into 10s windows → expect 3 parts.
	src := makeSilentMP3(t, tmp, 30)

	parts, err := splitAudioByDuration(context.Background(), src, tmp, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) < 2 {
		t.Fatalf("expected >=2 parts for 30s/10s split, got %d (%v)", len(parts), parts)
	}
	// Sorted + named seg-NNN.mp3
	for i, p := range parts {
		if filepath.Base(p) != fmtPartName(i) {
			t.Fatalf("part %d basename = %q, want %q", i, filepath.Base(p), fmtPartName(i))
		}
		if info, err := os.Stat(p); err != nil || info.Size() == 0 {
			t.Fatalf("part %d missing/empty: %v", i, err)
		}
	}
}

func TestSplitAudioByDuration_RejectsZero(t *testing.T) {
	_, err := splitAudioByDuration(context.Background(), "ignored", "ignored", 0)
	if err == nil {
		t.Fatal("expected error for segSeconds=0")
	}
}

func fmtPartName(i int) string {
	return fmt.Sprintf("seg-%03d.mp3", i)
}
