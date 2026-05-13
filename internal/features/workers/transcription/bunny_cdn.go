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

// bunnyPlayback bundles everything needed to build an HLS URL the CDN
// will actually serve: the system hostname, plus the pull zone's
// security settings used to sign URLs when token auth is enabled.
type bunnyPlayback struct {
	Hostname        string
	SecurityKey     string
	SecurityEnabled bool
}

// bunnyPlaybackCache memoizes (libraryID -> bunnyPlayback) so we hit
// the Bunny account API at most once per library per process lifetime.
// These attributes are immutable enough in practice that caching for the
// binary's lifetime is fine; restart the worker after rotating the zone
// security key in the Bunny dashboard.
var bunnyPlaybackCache sync.Map

// resolveBunnyPlayback turns a Stream `libraryID` into the public CDN
// hostname Bunny serves HLS playlists from + the security key needed to
// sign requests when token authentication is enabled on the pull zone.
//
// Resolution chain (both calls use the account-level BUNNY_API_KEY):
//
//	1. GET /videolibrary/{libraryID}  → PullZoneId
//	2. GET /pullzone/{pullZoneId}     → Hostnames[] + ZoneSecurityKey + ZoneSecurityEnabled
//	3. Pick the first IsSystemHostname entry (fallback: any *.b-cdn.net).
//	   Custom domains attached to the pull zone are skipped — their TLS
//	   cert may not be trusted by ffmpeg.
//
// The per-tenant Stream API key (`Tenant.bunnyLibraryApiKey`) cannot
// reach these endpoints; they live on a different host (api.bunny.net)
// and require the account key.
func (f *Feature) resolveBunnyPlayback(ctx context.Context, libraryID string) (*bunnyPlayback, error) {
	if libraryID == "" {
		return nil, fmt.Errorf("libraryID is required")
	}
	if v, ok := bunnyPlaybackCache.Load(libraryID); ok {
		return v.(*bunnyPlayback), nil
	}
	if f.bunnyAccountAPIKey == "" {
		return nil, fmt.Errorf("BUNNY_API_KEY not configured — required to resolve playback for library %s", libraryID)
	}

	pullZoneID, err := f.fetchPullZoneID(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	pb, err := f.fetchPullZonePlayback(ctx, pullZoneID)
	if err != nil {
		return nil, err
	}

	bunnyPlaybackCache.Store(libraryID, pb)
	return pb, nil
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
	ID                  int                     `json:"Id"`
	Hostnames           []bunnyPullZoneHostname `json:"Hostnames"`
	ZoneSecurityEnabled bool                    `json:"ZoneSecurityEnabled"`
	ZoneSecurityKey     string                  `json:"ZoneSecurityKey"`
}

func (f *Feature) fetchPullZonePlayback(ctx context.Context, pullZoneID int) (*bunnyPlayback, error) {
	endpoint := fmt.Sprintf("%s/pullzone/%d", bunnyAccountAPIBase, pullZoneID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build pullzone request: %w", err)
	}
	req.Header.Set("AccessKey", f.bunnyAccountAPIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bunny pullzone http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bunny pullzone status=%d body=%s", resp.StatusCode, string(body))
	}

	var pz bunnyPullZone
	if err := json.NewDecoder(resp.Body).Decode(&pz); err != nil {
		return nil, fmt.Errorf("decode pullzone: %w", err)
	}
	if len(pz.Hostnames) == 0 {
		return nil, fmt.Errorf("pullzone %d has no Hostnames in response", pullZoneID)
	}

	// Prefer the system hostname Bunny provisions automatically; falls back
	// to any *.b-cdn.net entry if IsSystemHostname is missing/false.
	var host string
	for _, h := range pz.Hostnames {
		if h.IsSystemHostname && h.Value != "" {
			host = h.Value
			break
		}
	}
	if host == "" {
		for _, h := range pz.Hostnames {
			if strings.HasSuffix(h.Value, ".b-cdn.net") {
				host = h.Value
				break
			}
		}
	}
	if host == "" {
		return nil, fmt.Errorf("pullzone %d has no system / b-cdn.net hostname (got %+v)", pullZoneID, pz.Hostnames)
	}
	if pz.ZoneSecurityEnabled && pz.ZoneSecurityKey == "" {
		return nil, fmt.Errorf("pullzone %d has ZoneSecurityEnabled=true but ZoneSecurityKey is empty", pullZoneID)
	}
	return &bunnyPlayback{
		Hostname:        host,
		SecurityKey:     pz.ZoneSecurityKey,
		SecurityEnabled: pz.ZoneSecurityEnabled,
	}, nil
}
