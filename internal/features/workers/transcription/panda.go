package transcription

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// Panda Video API base URL. Like the Bunny meta client in bunny_download.go
// we keep this dependency slice-local: read-only subtitle fetches don't
// justify a ports/adapters layer.
const defaultPandaBaseURL = "https://api-v2.pandavideo.com.br"

// pandaMaxSubtitleBytes caps how much of a subtitle response we buffer. A
// VTT file is KBs; anything past this is a broken/unexpected response.
const pandaMaxSubtitleBytes = 20 * 1024 * 1024 // 20 MB

// pandaVideoIDFromURL extracts the Panda video id from a lesson.mediaUrl
// embed URL of the form
//
//	https://player-vz-<hash>.tv.pandavideo.com.br/embed/?v=<uuid>
//
// The host may be any subdomain of pandavideo.com.br or pandavideo.com
// (player-vz-*.tv., player., dashboard.). The video id is read from the
// `v` query param; if absent, we fall back to the last non-empty path
// segment when it parses as a UUID.
func pandaVideoIDFromURL(mediaURL string) (string, error) {
	u, err := url.Parse(mediaURL)
	if err != nil {
		return "", fmt.Errorf("parse panda url: %w", err)
	}
	if !isPandaHost(u.Host) {
		return "", fmt.Errorf("not a Panda Video URL (host=%s)", u.Host)
	}

	if v := u.Query().Get("v"); v != "" {
		return v, nil
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) > 0 {
		last := segments[len(segments)-1]
		if last != "" {
			if _, err := uuid.Parse(last); err == nil {
				return last, nil
			}
		}
	}

	return "", fmt.Errorf("no video id found in Panda URL: %s", mediaURL)
}

// isPandaURL is a cheap dispatch check used by executeJob and the enqueue
// handler to decide whether a lesson's mediaUrl should take the Panda path.
// It parses the URL and checks the host — NOT a strings.Contains on the
// raw string, since a Bunny (or any other) URL could carry "pandavideo" in
// a query value without actually being hosted there.
func isPandaURL(mediaURL string) bool {
	u, err := url.Parse(mediaURL)
	if err != nil {
		return false
	}
	return isPandaHost(u.Host)
}

// isPandaHost reports whether host is (a subdomain of) pandavideo.com.br or
// pandavideo.com.
func isPandaHost(host string) bool {
	// url.URL.Host may carry a port; strip it before comparing labels.
	// net.SplitHostPort errors when there's no port — that's the common
	// case here, so just fall back to the original value.
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(host)
	return host == "pandavideo.com.br" || strings.HasSuffix(host, ".pandavideo.com.br") ||
		host == "pandavideo.com" || strings.HasSuffix(host, ".pandavideo.com")
}

// pandaVideoListItem is the subset of GET /videos entries we need to map
// player URLs (which carry video_external_id) to internal API ids.
type pandaVideoListItem struct {
	ID         string `json:"id"`
	ExternalID string `json:"video_external_id"`
}

type pandaVideoListResponse struct {
	Videos []pandaVideoListItem `json:"videos"`
	Pages  int                  `json:"pages"`
	Total  int                  `json:"total"`
}

// pandaVideosPageLimit is the page size for GET /videos scans; at 100 the
// whole account (~400 videos today) resolves in a handful of requests.
const pandaVideosPageLimit = 100

// pandaVideosMaxPages hard-caps the scan (100 pages × 100 videos = 10k).
// Past that the failure should be loud, not an endless crawl.
const pandaVideosMaxPages = 100

// fetchPandaVideosPage fetches one page of the account's video list.
func (f *Feature) fetchPandaVideosPage(ctx context.Context, page int) (*pandaVideoListResponse, error) {
	endpoint := fmt.Sprintf("%s/videos?limit=%d&page=%d",
		strings.TrimRight(f.pandaBaseURL, "/"), pandaVideosPageLimit, page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build panda videos request: %w", err)
	}
	req.Header.Set("Authorization", f.pandaAPIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("panda videos http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, pandaMaxSubtitleBytes))
		return nil, fmt.Errorf("panda videos status=%d body=%s", resp.StatusCode, string(body))
	}

	var parsed pandaVideoListResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("panda videos decode: %w", err)
	}
	return &parsed, nil
}

// resolvePandaInternalID maps the id carried by a player URL — the
// video_external_id — to the account's internal video id, which is the only
// id GET /subtitles/{video_id} accepts. Panda has no external-id lookup
// endpoint, so on a cache miss we scan the paginated GET /videos list and
// cache the whole map. The mutex only guards cache reads/swaps, never the
// HTTP scan itself: a duplicate concurrent scan is cheaper than serializing
// both workers behind network I/O.
func (f *Feature) resolvePandaInternalID(ctx context.Context, urlVideoID string) (string, error) {
	f.pandaIDCacheMu.Lock()
	if id, ok := f.pandaIDCache[urlVideoID]; ok {
		f.pandaIDCacheMu.Unlock()
		return id, nil
	}
	f.pandaIDCacheMu.Unlock()

	fresh := make(map[string]string)
	seen := 0
	for page := 1; page <= pandaVideosMaxPages; page++ {
		resp, err := f.fetchPandaVideosPage(ctx, page)
		if err != nil {
			return "", err
		}
		if len(resp.Videos) == 0 {
			// Guards a lying Pages value: an empty page means we're done.
			break
		}
		for _, v := range resp.Videos {
			if v.ExternalID != "" {
				fresh[v.ExternalID] = v.ID
			}
			// A URL that already carries an internal id resolves to itself.
			fresh[v.ID] = v.ID
			seen++
		}
		if page >= resp.Pages {
			break
		}
	}

	f.pandaIDCacheMu.Lock()
	f.pandaIDCache = fresh
	id, ok := f.pandaIDCache[urlVideoID]
	f.pandaIDCacheMu.Unlock()
	if !ok {
		return "", fmt.Errorf(
			"video %s not found in Panda account (checked %d videos) — check that PANDA_API_KEY belongs to the account hosting this video",
			urlVideoID, seen)
	}
	return id, nil
}

// pandaSubtitleTrack mirrors one entry of the GET /subtitles/{video_id}
// response.
type pandaSubtitleTrack struct {
	SrcLang       string `json:"srclang"`
	Label         string `json:"label"`
	Hidden        bool   `json:"hidden"`
	IsUploaded    bool   `json:"is_uploaded"`
	Transcription bool   `json:"transcription"`
}

type pandaSubtitlesResponse struct {
	Subtitles []pandaSubtitleTrack `json:"subtitles"`
}

// fetchPandaSubtitleTracks lists the subtitle/transcription tracks
// available for a Panda video.
func (f *Feature) fetchPandaSubtitleTracks(ctx context.Context, videoID string) ([]pandaSubtitleTrack, error) {
	endpoint := strings.TrimRight(f.pandaBaseURL, "/") + "/subtitles/" + videoID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build panda subtitles request: %w", err)
	}
	req.Header.Set("Authorization", f.pandaAPIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("panda subtitles http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, pandaMaxSubtitleBytes))
		return nil, fmt.Errorf("panda subtitles status=%d body=%s", resp.StatusCode, string(body))
	}

	var parsed pandaSubtitlesResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("panda subtitles decode: %w", err)
	}
	return parsed.Subtitles, nil
}

// pickPandaTrack chooses the best subtitle track to transcribe from:
// prefer a pt* track (the catalog is Brazilian Portuguese), otherwise
// take the first track available. Content in any language still gets
// embedded correctly by OpenAI, so falling back to non-pt is fine.
func pickPandaTrack(tracks []pandaSubtitleTrack) (pandaSubtitleTrack, error) {
	if len(tracks) == 0 {
		return pandaSubtitleTrack{}, fmt.Errorf(
			"no subtitle tracks — generate the transcription in the Panda dashboard (or via Panda aiworkflow) and re-enqueue")
	}
	for _, t := range tracks {
		if strings.HasPrefix(strings.ToLower(t.SrcLang), "pt") {
			return t, nil
		}
	}
	return tracks[0], nil
}

// fetchPandaVTT downloads the raw WebVTT body for a video's subtitle
// track in the given srclang.
func (f *Feature) fetchPandaVTT(ctx context.Context, videoID, srclang string) (string, error) {
	endpoint := strings.TrimRight(f.pandaBaseURL, "/") + "/subtitles/" + videoID + "/" + srclang
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build panda vtt request: %w", err)
	}
	req.Header.Set("Authorization", f.pandaAPIKey)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("panda vtt http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, pandaMaxSubtitleBytes))
	if err != nil {
		return "", fmt.Errorf("panda vtt read: %w", err)
	}

	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("no subtitle for language %q (video=%s, status=%d)", srclang, videoID, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("panda vtt status=%d body=%s", resp.StatusCode, string(body))
	}

	return string(body), nil
}

// resolvePandaTranscript downloads the video's best subtitle track and
// parses it into Whisper-shaped segments. No audio, no Whisper cost.
func (f *Feature) resolvePandaTranscript(ctx context.Context, mediaURL string) (segs []whisperSegment, text string, duration float64, language string, err error) {
	videoID, err := pandaVideoIDFromURL(mediaURL)
	if err != nil {
		return nil, "", 0, "", err
	}

	// The URL carries the video_external_id; the subtitles API only accepts
	// the account's internal id, so resolve before any subtitle call.
	videoID, err = f.resolvePandaInternalID(ctx, videoID)
	if err != nil {
		return nil, "", 0, "", err
	}

	tracks, err := f.fetchPandaSubtitleTracks(ctx, videoID)
	if err != nil {
		return nil, "", 0, "", err
	}

	track, err := pickPandaTrack(tracks)
	if err != nil {
		return nil, "", 0, "", err
	}

	vtt, err := f.fetchPandaVTT(ctx, videoID, track.SrcLang)
	if err != nil {
		return nil, "", 0, "", err
	}

	segs, err = parseVTT(vtt)
	if err != nil {
		return nil, "", 0, "", err
	}

	var b strings.Builder
	for i, s := range segs {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strings.TrimSpace(s.Text))
	}

	language = track.SrcLang
	if language == "" {
		language = "pt"
	}

	return segs, strings.TrimSpace(b.String()), segs[len(segs)-1].End, language, nil
}

// ---------- WebVTT parsing ----------

// vttTagRegexp strips inline cue tags (<c>, <v Speaker>, <i>, timestamp
// tags like <00:00:01.000>) — anything between angle brackets.
var vttTagRegexp = regexp.MustCompile(`<[^>]*>`)

// vttTimingRegexp matches a cue timing line, accepting both
// HH:MM:SS.mmm --> HH:MM:SS.mmm and the short MM:SS.mmm form, ignoring any
// cue settings (align:start position:0%, etc.) trailing the end timestamp.
var vttTimingRegexp = regexp.MustCompile(
	`^\s*((?:\d+:)?\d{2}:\d{2}[.,]\d{3})\s*-->\s*((?:\d+:)?\d{2}:\d{2}[.,]\d{3})`)

// parseVTT converts a WebVTT document into whisperSegment-shaped cues so
// the existing chunker (which only knows Whisper's output shape) can
// process Panda subtitles unchanged.
func parseVTT(vtt string) ([]whisperSegment, error) {
	// Strip a UTF-8 BOM if present.
	vtt = strings.TrimPrefix(vtt, "\uFEFF")
	// Normalize line endings.
	vtt = strings.ReplaceAll(vtt, "\r\n", "\n")
	vtt = strings.ReplaceAll(vtt, "\r", "\n")

	lines := strings.Split(vtt, "\n")

	var segments []whisperSegment
	i := 0

	// Skip the WEBVTT header line (optionally followed by trailing text)
	// if present.
	if i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "WEBVTT") {
		i++
	}

	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			continue
		}

		// Skip NOTE / STYLE / REGION blocks: consume until the next blank
		// line.
		if strings.HasPrefix(line, "NOTE") || line == "STYLE" || line == "REGION" ||
			strings.HasPrefix(line, "STYLE ") || strings.HasPrefix(line, "REGION ") {
			for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
				i++
			}
			continue
		}

		// Optional cue identifier line: a line that is NOT a timing line
		// but is immediately followed by one.
		if !vttTimingRegexp.MatchString(line) {
			if i+1 < len(lines) && vttTimingRegexp.MatchString(lines[i+1]) {
				i++ // skip cue identifier
				line = strings.TrimSpace(lines[i])
			} else {
				// Not a cue block we understand; skip this line.
				i++
				continue
			}
		}

		m := vttTimingRegexp.FindStringSubmatch(line)
		if m == nil {
			i++
			continue
		}
		start, errStart := parseVTTTimestamp(m[1])
		end, errEnd := parseVTTTimestamp(m[2])
		i++
		if errStart != nil || errEnd != nil {
			continue
		}

		var textLines []string
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
			textLines = append(textLines, vttTagRegexp.ReplaceAllString(strings.TrimSpace(lines[i]), ""))
			i++
		}
		text := strings.TrimSpace(strings.Join(textLines, " "))
		if text == "" {
			continue
		}

		segments = append(segments, whisperSegment{Start: start, End: end, Text: text})
	}

	if len(segments) == 0 {
		return nil, fmt.Errorf("VTT parsed to 0 cues")
	}
	return segments, nil
}

// parseVTTTimestamp parses HH:MM:SS.mmm or MM:SS.mmm (comma decimal
// separator also tolerated) into seconds.
func parseVTTTimestamp(ts string) (float64, error) {
	ts = strings.ReplaceAll(ts, ",", ".")
	parts := strings.Split(ts, ":")

	var h, m int
	var s float64
	var err error

	switch len(parts) {
	case 3:
		h, err = strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("parse vtt hours: %w", err)
		}
		m, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("parse vtt minutes: %w", err)
		}
		s, err = strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return 0, fmt.Errorf("parse vtt seconds: %w", err)
		}
	case 2:
		m, err = strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("parse vtt minutes: %w", err)
		}
		s, err = strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, fmt.Errorf("parse vtt seconds: %w", err)
		}
	default:
		return 0, fmt.Errorf("unrecognized vtt timestamp: %q", ts)
	}

	return float64(h*3600+m*60) + s, nil
}
