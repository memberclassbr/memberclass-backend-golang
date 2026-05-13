package transcription

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Bunny Stream API base URL. Owned by the slice (the existing BunnyService
// is used for upload flows; here we only need read-only metadata, so we
// keep the dependency narrow rather than extending the service interface).
const defaultBunnyBaseURL = "https://video.bunnycdn.com/library"

// iframeMediaDeliveryDomain hosts both the embed player AND a direct HLS
// playlist URL (`<domain>/<libraryId>/<videoId>/playlist.m3u8`) that
// ffmpeg can read without a custom CDN hostname or account-level API key.
// This works as long as the library does NOT enforce token authentication
// on direct playlist access (the default for most libraries).
const iframeMediaDeliveryDomain = "iframe.mediadelivery.net"

// bunnyVideoStatus codes (from the Stream API):
//   0 Created · 1 Uploaded · 2 Processing · 3 Transcoding · 4 Finished
//   5 Error · 6 UploadFailed · 7 JitSegmenting · 8 JitPlaylistsCreated
// We only transcribe Finished videos (status=4).
const bunnyVideoStatusFinished = 4

type bunnyVideoMeta struct {
	GUID           string  `json:"guid"`
	Status         int     `json:"status"`
	Length         float64 `json:"length"` // seconds
	VideoLibraryID int     `json:"videoLibraryId"`
	Title          string  `json:"title"`
}

// guidFromEmbedURL parses an iframe URL of the form
//
//	https://iframe.mediadelivery.net/embed/{libraryId}/{guid}[?query...]
//
// returning (libraryId, guid). The lesson.mediaUrl column carries this
// format with a query string of player options that we simply discard.
func guidFromEmbedURL(embedURL string) (string, string, error) {
	u, err := url.Parse(embedURL)
	if err != nil {
		return "", "", fmt.Errorf("parse embed url: %w", err)
	}
	if u.Host != iframeMediaDeliveryDomain {
		return "", "", fmt.Errorf("not a Bunny iframe URL (host=%s)", u.Host)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "embed" {
		return "", "", fmt.Errorf("not a Bunny embed URL: %s", embedURL)
	}
	libraryID := parts[1]
	guid := parts[2]
	if libraryID == "" || guid == "" {
		return "", "", fmt.Errorf("empty libraryId/guid in %s", embedURL)
	}
	return libraryID, guid, nil
}

// fetchBunnyVideoMeta calls the Stream API to validate that the video has
// finished processing (status=4). Returns the metadata or an error
// describing why the video is unprocessable.
func (f *Feature) fetchBunnyVideoMeta(ctx context.Context, libraryID, guid, accessKey string) (*bunnyVideoMeta, error) {
	if libraryID == "" || guid == "" {
		return nil, fmt.Errorf("libraryID and guid are required")
	}
	if accessKey == "" {
		return nil, fmt.Errorf("Bunny library access key is required")
	}

	endpoint := strings.TrimRight(f.bunnyBaseURL, "/") + "/" + libraryID + "/videos/" + guid
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build bunny meta request: %w", err)
	}
	req.Header.Set("AccessKey", accessKey)
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bunny meta http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bunny meta status=%d body=%s", resp.StatusCode, string(body))
	}

	var meta bunnyVideoMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, fmt.Errorf("bunny meta decode: %w", err)
	}
	if meta.Status != bunnyVideoStatusFinished {
		return nil, fmt.Errorf("bunny video not finished (status=%d, guid=%s)", meta.Status, guid)
	}
	return &meta, nil
}

// buildHLSURL returns the publicly-readable HLS playlist URL for a Bunny
// video. We rely on the iframe.mediadelivery.net direct-playlist pattern,
// which works for any library that has not enabled token authentication.
// Libraries with token auth need a different code path (signed URLs); the
// pipeline surfaces the resulting ffmpeg 401 in jobs.error so the operator
// can disable token auth or extend this resolver.
func buildHLSURL(libraryID, guid string) string {
	return fmt.Sprintf("https://%s/%s/%s/playlist.m3u8",
		iframeMediaDeliveryDomain, libraryID, guid)
}
