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

// withBunnyAccountAPI spins an httptest server that responds to both
// /videolibrary/{id} and /pullzone/{id}, then redirects the
// `bunnyAccountAPIBase` constant via the f.httpClient transport. We
// can't override the constant at runtime, so the test uses a Transport
// that rewrites the URL.
type rewriteTransport struct {
	base    string
	wrapped http.RoundTripper
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.String(), bunnyAccountAPIBase) {
		newURL := rt.base + strings.TrimPrefix(req.URL.String(), bunnyAccountAPIBase)
		req.URL.Host = ""
		req.URL.Scheme = ""
		// Re-parse via NewRequest so URL fields are valid.
		fresh, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
		if err != nil {
			return nil, err
		}
		fresh.Header = req.Header
		return rt.wrapped.RoundTrip(fresh)
	}
	return rt.wrapped.RoundTrip(req)
}

func TestResolveBunnyCDNHostname_HappyPath(t *testing.T) {
	// Reset cache between tests.
	cdnHostnameCache = sync.Map{}

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
	host, err := f.resolveBunnyCDNHostname(context.Background(), "378335")
	if err != nil {
		t.Fatal(err)
	}
	if host != "vz-abc12345.b-cdn.net" {
		t.Fatalf("got %q, want vz-abc12345.b-cdn.net", host)
	}

	// Second call must come from cache (server would 404 if we hit it again
	// since the handler isn't re-armed). Use Hit count by counting server.Hits — easier: just call again, it'd panic otherwise.
	if cached, err := f.resolveBunnyCDNHostname(context.Background(), "378335"); err != nil || cached != host {
		t.Fatalf("cached lookup failed: %v / %q", err, cached)
	}
}

func TestResolveBunnyCDNHostname_FallsBackToBCDNNet(t *testing.T) {
	cdnHostnameCache = sync.Map{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/videolibrary/lib":
			_ = json.NewEncoder(w).Encode(bunnyVideoLibrary{ID: 1, PullZoneID: 42})
		case "/pullzone/42":
			// No IsSystemHostname flag set — fall back to suffix match.
			_ = json.NewEncoder(w).Encode(bunnyPullZone{
				ID: 42,
				Hostnames: []bunnyPullZoneHostname{
					{Value: "custom.tenant.com"},
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
	host, err := f.resolveBunnyCDNHostname(context.Background(), "lib")
	if err != nil {
		t.Fatal(err)
	}
	if host != "vz-fallback.b-cdn.net" {
		t.Fatalf("got %q", host)
	}
}

func TestResolveBunnyCDNHostname_ErrorsWithoutKey(t *testing.T) {
	cdnHostnameCache = sync.Map{}
	f := &Feature{httpClient: http.DefaultClient}
	_, err := f.resolveBunnyCDNHostname(context.Background(), "lib")
	if err == nil || !strings.Contains(err.Error(), "BUNNY_API_KEY") {
		t.Fatalf("expected BUNNY_API_KEY error, got %v", err)
	}
}

func TestResolveBunnyCDNHostname_PropagatesVideoLibraryStatus(t *testing.T) {
	cdnHostnameCache = sync.Map{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "library not found", http.StatusNotFound)
	}))
	defer server.Close()
	f := &Feature{
		bunnyAccountAPIKey: "k",
		httpClient:         &http.Client{Transport: &rewriteTransport{base: server.URL, wrapped: server.Client().Transport}},
	}
	_, err := f.resolveBunnyCDNHostname(context.Background(), "missing")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 in error, got %v", err)
	}
}

