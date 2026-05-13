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

// ffmpegProtocolWhitelist locks ffmpeg's URL handlers down to the
// protocols this pipeline actually needs. Without it, ffmpeg accepts
// `file://`, `concat:`, `rtmp:`, `srtp:`, `sftp:`, etc., which would be
// a serious SSRF / local-file-read primitive if the input string ever
// became attacker-influenced. Today the input is built from Bunny API
// responses, but constraining the surface is cheap defense-in-depth.
const ffmpegProtocolWhitelist = "file,http,https,tls,tcp,crypto,hls,rtp,udp"

// hlsAllowedExtensions is the upstream alpine ffmpeg 8 default list +
// `dts`. ffmpeg 7 split the single legacy `-allowed_extensions` option
// into two separate ones — `-allowed_extensions` for sub-playlists,
// and `-allowed_segment_extensions` for the actual media segments.
// The production error specifically references the segment list
// ("is not in allowed_segment_extensions"), so we pass BOTH flags
// with the same value to keep them in sync.
//
// Bunny serves some libraries' audio-only HLS variants as `.dts` (DTS
// Coherent Acoustics) segments. The stock ffmpeg 8 default doesn't
// include `dts`, hence the explicit add.
//
// The list is the alpine ffmpeg 8 default merged with `dts`; lifting
// the upstream defaults verbatim avoids regressing libraries that ship
// .m3u8 sub-playlists, .mpegts, fragmented mp4 (.cmfv/.cmfa/.fmp4),
// subtitle tracks (.vtt/.webvtt), etc.
const hlsAllowedExtensions = "3gp,aac,avi,ac3,eac3,flac,mkv,m3u8,m4a,m4s,m4v,mpg,mov,mp2,mp3,mp4,mpeg,mpegts,ogg,ogv,oga,ts,vob,vtt,wav,webvtt,cmfv,cmfa,ec3,fmp4,dts"

// extractAudioMP3 invokes ffmpeg to read an HLS playlist URL, MP4 URL, or
// local file from `input` and produce a mono 16 kHz MP3 at 64 kbps in
// outPath. Mono + 16 kHz are Whisper's sweet spot — same recognition
// quality as 44.1 kHz stereo at ~10% the bandwidth. Returns outPath on
// success.
//
// For http/https inputs the iframe.mediadelivery.net Referer header and
// the HLS segment-extension whitelist are added. Both flags are HLS-
// demuxer options that the file demuxer rejects, so they're gated on
// isHTTPURL.
func extractAudioMP3(ctx context.Context, input, outPath string) (string, error) {
	args := []string{
		"-y",
		"-loglevel", "error",
		"-protocol_whitelist", ffmpegProtocolWhitelist,
	}
	if isHTTPURL(input) {
		args = append(args,
			"-referer", bunnyIframeReferer,
			"-allowed_extensions", hlsAllowedExtensions,
			"-allowed_segment_extensions", hlsAllowedExtensions,
		)
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
		"-protocol_whitelist", ffmpegProtocolWhitelist,
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
