package transcription

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// rewriteTransport is a tiny http.RoundTripper that redirects requests
// hitting bunnyAccountAPIBase to a local httptest server. We can't override
// the constant at runtime, so we intercept and rewrite per request.
type rewriteTransport struct {
	base    string
	wrapped http.RoundTripper
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.String(), bunnyAccountAPIBase) {
		newURL := rt.base + strings.TrimPrefix(req.URL.String(), bunnyAccountAPIBase)
		fresh, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
		if err != nil {
			return nil, err
		}
		fresh.Header = req.Header
		return rt.wrapped.RoundTrip(fresh)
	}
	return rt.wrapped.RoundTrip(req)
}

func TestResolveBunnyPlayback_HappyPath_TokenAuthOff(t *testing.T) {
	bunnyPlaybackCache = sync.Map{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("AccessKey") != "account-key" {
			t.Fatalf("AccessKey = %q", r.Header.Get("AccessKey"))
		}
		switch r.URL.Path {
		case "/videolibrary/378335":
			_ = json.NewEncoder(w).Encode(bunnyVideoLibrary{ID: 378335, Name: "test", PullZoneID: 999})
		case "/pullzone/999":
			_ = json.NewEncoder(w).Encode(bunnyPullZone{
				ID: 999,
				Hostnames: []bunnyPullZoneHostname{
					{Value: "custom.tenant.com", IsSystemHostname: false},
					{Value: "vz-abc12345.b-cdn.net", IsSystemHostname: true},
				},
				ZoneSecurityEnabled: false,
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	f := &Feature{
		bunnyAccountAPIKey: "account-key",
		httpClient:         &http.Client{Transport: &rewriteTransport{base: server.URL, wrapped: server.Client().Transport}},
	}
	pb, err := f.resolveBunnyPlayback(context.Background(), "378335")
	if err != nil {
		t.Fatal(err)
	}
	if pb.Hostname != "vz-abc12345.b-cdn.net" || pb.SecurityEnabled {
		t.Fatalf("unexpected playback: %+v", pb)
	}
	// Cached on second call.
	if cached, err := f.resolveBunnyPlayback(context.Background(), "378335"); err != nil || cached != pb {
		t.Fatalf("cached lookup failed: %v / %+v", err, cached)
	}
}

func TestResolveBunnyPlayback_HappyPath_TokenAuthOn(t *testing.T) {
	bunnyPlaybackCache = sync.Map{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/videolibrary/378335":
			_ = json.NewEncoder(w).Encode(bunnyVideoLibrary{ID: 378335, PullZoneID: 999})
		case "/pullzone/999":
			_ = json.NewEncoder(w).Encode(bunnyPullZone{
				ID: 999,
				Hostnames: []bunnyPullZoneHostname{
					{Value: "vz-x.b-cdn.net", IsSystemHostname: true},
				},
				ZoneSecurityEnabled: true,
				ZoneSecurityKey:     "super-secret",
			})
		}
	}))
	defer server.Close()

	f := &Feature{
		bunnyAccountAPIKey: "k",
		httpClient:         &http.Client{Transport: &rewriteTransport{base: server.URL, wrapped: server.Client().Transport}},
	}
	pb, err := f.resolveBunnyPlayback(context.Background(), "378335")
	if err != nil {
		t.Fatal(err)
	}
	if !pb.SecurityEnabled || pb.SecurityKey != "super-secret" {
		t.Fatalf("expected security enabled w/ key, got %+v", pb)
	}
}

func TestResolveBunnyPlayback_RejectsSecurityEnabledWithoutKey(t *testing.T) {
	bunnyPlaybackCache = sync.Map{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/videolibrary/lib":
			_ = json.NewEncoder(w).Encode(bunnyVideoLibrary{ID: 1, PullZoneID: 42})
		case "/pullzone/42":
			_ = json.NewEncoder(w).Encode(bunnyPullZone{
				ID:                  42,
				Hostnames:           []bunnyPullZoneHostname{{Value: "vz-x.b-cdn.net", IsSystemHostname: true}},
				ZoneSecurityEnabled: true,
				ZoneSecurityKey:     "", // misconfigured
			})
		}
	}))
	defer server.Close()
	f := &Feature{
		bunnyAccountAPIKey: "k",
		httpClient:         &http.Client{Transport: &rewriteTransport{base: server.URL, wrapped: server.Client().Transport}},
	}
	_, err := f.resolveBunnyPlayback(context.Background(), "lib")
	if err == nil || !strings.Contains(err.Error(), "ZoneSecurityEnabled=true but ZoneSecurityKey is empty") {
		t.Fatalf("expected ZoneSecurityKey error, got %v", err)
	}
}

func TestResolveBunnyPlayback_FallsBackToBCDNNet(t *testing.T) {
	bunnyPlaybackCache = sync.Map{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/videolibrary/lib":
			_ = json.NewEncoder(w).Encode(bunnyVideoLibrary{ID: 1, PullZoneID: 42})
		case "/pullzone/42":
			_ = json.NewEncoder(w).Encode(bunnyPullZone{
				ID: 42,
				Hostnames: []bunnyPullZoneHostname{
					{Value: "custom.tenant.com"}, // no IsSystemHostname flag
					{Value: "vz-fallback.b-cdn.net"},
				},
			})
		}
	}))
	defer server.Close()
	f := &Feature{
		bunnyAccountAPIKey: "k",
		httpClient:         &http.Client{Transport: &rewriteTransport{base: server.URL, wrapped: server.Client().Transport}},
	}
	pb, err := f.resolveBunnyPlayback(context.Background(), "lib")
	if err != nil {
		t.Fatal(err)
	}
	if pb.Hostname != "vz-fallback.b-cdn.net" {
		t.Fatalf("got %q", pb.Hostname)
	}
}

func TestResolveBunnyPlayback_ErrorsWithoutKey(t *testing.T) {
	bunnyPlaybackCache = sync.Map{}
	f := &Feature{httpClient: http.DefaultClient}
	_, err := f.resolveBunnyPlayback(context.Background(), "lib")
	if err == nil || !strings.Contains(err.Error(), "BUNNY_API_KEY") {
		t.Fatalf("expected BUNNY_API_KEY error, got %v", err)
	}
}

func TestResolveBunnyPlayback_PropagatesVideoLibraryStatus(t *testing.T) {
	bunnyPlaybackCache = sync.Map{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "library not found", http.StatusNotFound)
	}))
	defer server.Close()
	f := &Feature{
		bunnyAccountAPIKey: "k",
		httpClient:         &http.Client{Transport: &rewriteTransport{base: server.URL, wrapped: server.Client().Transport}},
	}
	_, err := f.resolveBunnyPlayback(context.Background(), "missing")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 in error, got %v", err)
	}
}
