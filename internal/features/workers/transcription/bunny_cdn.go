package transcription

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// bunnyAccountAPIBase is the host for Bunny's account-level API. Distinct
// from the per-library Stream API at video.bunnycdn.com/library — the
// account API exposes pullzone + library settings the per-library key
// cannot reach.
const bunnyAccountAPIBase = "https://api.bunny.net"

// cdnHostnameCache memoizes (libraryID -> b-cdn.net hostname) so we hit
// the Bunny account API at most once per library per process lifetime.
// CDN hostnames are immutable once a library is created.
var cdnHostnameCache sync.Map

// resolveBunnyCDNHostname turns a Stream `libraryID` into the public CDN
// hostname Bunny serves HLS playlists from. Resolution chain:
//
//	1. GET /videolibrary/{libraryID}  → PullZoneId
//	2. GET /pullzone/{pullZoneId}     → Hostnames[]
//	3. Pick the first system hostname (or any *.b-cdn.net entry);
//	   custom domains tied to the pullzone are skipped because they may
//	   not have a TLS cert that ffmpeg trusts.
//
// Both calls authenticate with the account-level API key in BUNNY_API_KEY
// (single key org-wide; the per-tenant `bunnyLibraryApiKey` cannot reach
// these endpoints).
func (f *Feature) resolveBunnyCDNHostname(ctx context.Context, libraryID string) (string, error) {
	if libraryID == "" {
		return "", fmt.Errorf("libraryID is required")
	}
	if v, ok := cdnHostnameCache.Load(libraryID); ok {
		return v.(string), nil
	}
	if f.bunnyAccountAPIKey == "" {
		return "", fmt.Errorf("BUNNY_API_KEY not configured — required to resolve CDN hostname for library %s", libraryID)
	}

	pullZoneID, err := f.fetchPullZoneID(ctx, libraryID)
	if err != nil {
		return "", err
	}
	hostname, err := f.fetchPullZoneHostname(ctx, pullZoneID)
	if err != nil {
		return "", err
	}

	cdnHostnameCache.Store(libraryID, hostname)
	return hostname, nil
}

type bunnyVideoLibrary struct {
	ID         int    `json:"Id"`
	Name       string `json:"Name"`
	PullZoneID int    `json:"PullZoneId"`
}

func (f *Feature) fetchPullZoneID(ctx context.Context, libraryID string) (int, error) {
	endpoint := bunnyAccountAPIBase + "/videolibrary/" + libraryID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("build videolibrary request: %w", err)
	}
	req.Header.Set("AccessKey", f.bunnyAccountAPIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("bunny videolibrary http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("bunny videolibrary status=%d body=%s", resp.StatusCode, string(body))
	}

	var meta bunnyVideoLibrary
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return 0, fmt.Errorf("decode videolibrary: %w", err)
	}
	if meta.PullZoneID == 0 {
		return 0, fmt.Errorf("library %s has no PullZoneId in response", libraryID)
	}
	return meta.PullZoneID, nil
}

type bunnyPullZoneHostname struct {
	Value            string `json:"Value"`
	IsSystemHostname bool   `json:"IsSystemHostname"`
}

type bunnyPullZone struct {
	ID        int                     `json:"Id"`
	Hostnames []bunnyPullZoneHostname `json:"Hostnames"`
}

func (f *Feature) fetchPullZoneHostname(ctx context.Context, pullZoneID int) (string, error) {
	endpoint := fmt.Sprintf("%s/pullzone/%d", bunnyAccountAPIBase, pullZoneID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build pullzone request: %w", err)
	}
	req.Header.Set("AccessKey", f.bunnyAccountAPIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("bunny pullzone http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("bunny pullzone status=%d body=%s", resp.StatusCode, string(body))
	}

	var pz bunnyPullZone
	if err := json.NewDecoder(resp.Body).Decode(&pz); err != nil {
		return "", fmt.Errorf("decode pullzone: %w", err)
	}
	if len(pz.Hostnames) == 0 {
		return "", fmt.Errorf("pullzone %d has no Hostnames in response", pullZoneID)
	}
	// Prefer the system hostname Bunny provisions automatically; falls back
	// to any *.b-cdn.net entry if the IsSystemHostname flag is missing.
	for _, h := range pz.Hostnames {
		if h.IsSystemHostname && h.Value != "" {
			return h.Value, nil
		}
	}
	for _, h := range pz.Hostnames {
		if strings.HasSuffix(h.Value, ".b-cdn.net") {
			return h.Value, nil
		}
	}
	return "", fmt.Errorf("pullzone %d has no system / b-cdn.net hostname (got %+v)", pullZoneID, pz.Hostnames)
}
