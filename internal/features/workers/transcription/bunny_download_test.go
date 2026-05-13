package transcription

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestBuildHLSURL(t *testing.T) {
	got := buildHLSURL("383534", "4e8da26a-754a-4ba7-9df7-02a0e8f2f396")
	want := "https://iframe.mediadelivery.net/383534/4e8da26a-754a-4ba7-9df7-02a0e8f2f396/playlist.m3u8"
	if got != want {
		t.Fatalf("buildHLSURL = %q, want %q", got, want)
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
