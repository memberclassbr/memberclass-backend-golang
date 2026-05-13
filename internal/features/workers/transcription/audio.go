package transcription

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// bunnyIframeReferer is what Bunny's CDN edge checks for before serving
// HLS playlists, even when AllowedReferrers/BlockedReferrers are empty
// and AllowDirectPlay=true. Empirically, requests without this Referer
// header come back as 403 Forbidden. The iframe player normally sets it
// automatically; ffmpeg has to be told.
const bunnyIframeReferer = "https://iframe.mediadelivery.net/"

// extractAudioMP3 invokes ffmpeg to read an HLS playlist URL, MP4 URL, or
// local file from `input` and produce a mono 16 kHz MP3 at 64 kbps in
// outPath. Mono + 16 kHz are Whisper's sweet spot — same recognition
// quality as 44.1 kHz stereo at ~10% the bandwidth. Returns outPath on
// success.
//
// For http/https inputs the iframe.mediadelivery.net Referer header is
// added — Bunny's CDN edge returns 403 without it even when the pull
// zone's referrer allowlist is empty. The flag is omitted for local
// inputs because the file demuxer rejects unknown HTTP options.
func extractAudioMP3(ctx context.Context, input, outPath string) (string, error) {
	args := []string{"-y", "-loglevel", "error"}
	if isHTTPURL(input) {
		args = append(args, "-referer", bunnyIframeReferer)
	}
	args = append(args,
		"-i", input,
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-acodec", "libmp3lame",
		"-ab", "64k",
		outPath,
	)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg extract: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return outPath, nil
}

func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// splitAudioByDuration slices `src` into N parts of `segSeconds` seconds
// each using ffmpeg's segment muxer. `-c copy` re-uses the existing MP3
// frames, so the operation is fast (no re-encode). The output filenames
// are deterministic — `<outDir>/seg-NNN.mp3` — and returned sorted.
//
// Whisper's API caps uploads at 25 MB; the pipeline uses this when total
// audio exceeds the safety bytes threshold. For a 64 kbps mono MP3, 10
// minutes is ~4.7 MB — five 10-minute parts comfortably fit five Whisper
// calls without hitting the cap.
func splitAudioByDuration(ctx context.Context, src, outDir string, segSeconds int) ([]string, error) {
	if segSeconds <= 0 {
		return nil, fmt.Errorf("segSeconds must be > 0, got %d", segSeconds)
	}
	pattern := filepath.Join(outDir, "seg-%03d.mp3")
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y",
		"-loglevel", "error",
		"-i", src,
		"-f", "segment",
		"-segment_time", strconv.Itoa(segSeconds),
		"-c", "copy",
		pattern,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg split: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		return nil, fmt.Errorf("read split outdir: %w", err)
	}
	var parts []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "seg-") || !strings.HasSuffix(name, ".mp3") {
			continue
		}
		parts = append(parts, filepath.Join(outDir, name))
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("ffmpeg segment muxer produced no parts")
	}
	// ReadDir order is unspecified across filesystems; sort to keep the
	// transcribed-text concatenation deterministic.
	sort.Strings(parts)
	return parts, nil
}
