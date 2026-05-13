package transcription

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGuidFromEmbedURL_HappyPath(t *testing.T) {
	libID, guid, err := guidFromEmbedURL(
		"https://iframe.mediadelivery.net/embed/383534/4e8da26a-754a-4ba7-9df7-02a0e8f2f396?autoplay=false&loop=false",
	)
	if err != nil {
		t.Fatal(err)
	}
	if libID != "383534" {
		t.Fatalf("libID = %q", libID)
	}
	if guid != "4e8da26a-754a-4ba7-9df7-02a0e8f2f396" {
		t.Fatalf("guid = %q", guid)
	}
}

func TestGuidFromEmbedURL_RejectsNonBunnyHost(t *testing.T) {
	if _, _, err := guidFromEmbedURL("https://example.com/embed/1/2"); err == nil {
		t.Fatal("expected error for non-Bunny host")
	}
}

func TestGuidFromEmbedURL_RejectsMalformedPath(t *testing.T) {
	cases := []string{
		"https://iframe.mediadelivery.net/embed/383534",            // missing guid
		"https://iframe.mediadelivery.net/play/383534/abc",         // wrong prefix
		"https://iframe.mediadelivery.net/",                        // empty
	}
	for _, c := range cases {
		if _, _, err := guidFromEmbedURL(c); err == nil {
			t.Fatalf("expected error for %q", c)
		}
	}
}

func TestBuildHLSURL_NoTokenAuth(t *testing.T) {
	guid := "4e8da26a-754a-4ba7-9df7-02a0e8f2f396"
	want := "https://vz-abc12345.b-cdn.net/" + guid + "/playlist.m3u8"

	pb := &bunnyPlayback{Hostname: "vz-abc12345.b-cdn.net"}
	if got := buildHLSURL(pb, guid, time.Unix(0, 0), time.Hour); got != want {
		t.Fatalf("plain: got %q, want %q", got, want)
	}
	// Tolerant of scheme/trailing slash on the hostname input.
	if got := buildHLSURL(&bunnyPlayback{Hostname: "https://vz-abc12345.b-cdn.net/"}, guid, time.Unix(0, 0), time.Hour); got != want {
		t.Fatalf("scheme+slash: got %q, want %q", got, want)
	}
}

func TestBuildHLSURL_WithTokenAuth(t *testing.T) {
	guid := "abc"
	pb := &bunnyPlayback{
		Hostname:        "vz-x.b-cdn.net",
		SecurityKey:     "secret",
		SecurityEnabled: true,
	}
	// Fixed time so the token is deterministic.
	now := time.Unix(1700000000, 0)
	got := buildHLSURL(pb, guid, now, time.Hour)

	// Verify shape: contains token, token_path, expires.
	if !strings.Contains(got, "?token=") {
		t.Fatalf("missing ?token= in %q", got)
	}
	if !strings.Contains(got, "token_path=") {
		t.Fatalf("missing token_path= in %q", got)
	}
	expires := strconv.FormatInt(now.Add(time.Hour).Unix(), 10)
	if !strings.Contains(got, "expires="+expires) {
		t.Fatalf("missing expires in %q", got)
	}
	// Re-derive token and assert exact value.
	tokenPath := "/" + guid + "/"
	sum := sha256.Sum256([]byte("secret" + tokenPath + expires))
	wantToken := strings.TrimRight(base64.URLEncoding.EncodeToString(sum[:]), "=")
	if !strings.Contains(got, "token="+wantToken+"&") {
		t.Fatalf("token mismatch in %q (want %q)", got, wantToken)
	}
}

func TestFetchBunnyVideoMeta_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("AccessKey") != "tenant-key" {
			t.Fatalf("AccessKey = %q", r.Header.Get("AccessKey"))
		}
		if !strings.HasSuffix(r.URL.Path, "/lib-1/videos/abc-123") {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(bunnyVideoMeta{
			GUID: "abc-123", Status: 4, Length: 123.4, VideoLibraryID: 1, Title: "Aula 1",
		})
	}))
	defer server.Close()

	f := &Feature{
		bunnyBaseURL: server.URL,
		httpClient:   server.Client(),
	}
	meta, err := f.fetchBunnyVideoMeta(context.Background(), "lib-1", "abc-123", "tenant-key")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Status != 4 || meta.Length != 123.4 {
		t.Fatalf("unexpected meta: %+v", meta)
	}
}

func TestFetchBunnyVideoMeta_RejectsUnfinished(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(bunnyVideoMeta{GUID: "g", Status: 2})
	}))
	defer server.Close()

	f := &Feature{bunnyBaseURL: server.URL, httpClient: server.Client()}
	_, err := f.fetchBunnyVideoMeta(context.Background(), "lib-1", "g", "k")
	if err == nil || !strings.Contains(err.Error(), "not finished") {
		t.Fatalf("expected 'not finished' error, got %v", err)
	}
}

func TestFetchBunnyVideoMeta_PropagatesHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "video not found", http.StatusNotFound)
	}))
	defer server.Close()

	f := &Feature{bunnyBaseURL: server.URL, httpClient: server.Client()}
	_, err := f.fetchBunnyVideoMeta(context.Background(), "lib", "missing", "k")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 error, got %v", err)
	}
}

func TestFetchBunnyVideoMeta_RejectsEmptyArgs(t *testing.T) {
	f := &Feature{bunnyBaseURL: "https://example.invalid", httpClient: http.DefaultClient}
	if _, err := f.fetchBunnyVideoMeta(context.Background(), "", "g", "k"); err == nil {
		t.Fatal("expected error for empty libraryID")
	}
	if _, err := f.fetchBunnyVideoMeta(context.Background(), "lib", "", "k"); err == nil {
		t.Fatal("expected error for empty guid")
	}
	if _, err := f.fetchBunnyVideoMeta(context.Background(), "lib", "g", ""); err == nil {
		t.Fatal("expected error for empty accessKey")
	}
}
