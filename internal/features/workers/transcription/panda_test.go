package transcription

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestPandaVideoIDFromURL_HappyPath(t *testing.T) {
	id, err := pandaVideoIDFromURL("https://player-vz-464ed8d8-7b7.tv.pandavideo.com.br/embed/?v=18d69307-e9af-4d1a-8891-77f88794ce43")
	if err != nil {
		t.Fatal(err)
	}
	if id != "18d69307-e9af-4d1a-8891-77f88794ce43" {
		t.Fatalf("id = %q", id)
	}
}

func TestPandaVideoIDFromURL_ExtraQueryParams(t *testing.T) {
	id, err := pandaVideoIDFromURL("https://player-vz-464ed8d8-7b7.tv.pandavideo.com.br/embed/?v=18d69307-e9af-4d1a-8891-77f88794ce43&autoplay=true&loop=false")
	if err != nil {
		t.Fatal(err)
	}
	if id != "18d69307-e9af-4d1a-8891-77f88794ce43" {
		t.Fatalf("id = %q", id)
	}
}

func TestPandaVideoIDFromURL_AlternateHost(t *testing.T) {
	id, err := pandaVideoIDFromURL("https://player.pandavideo.com.br/embed/?v=18d69307-e9af-4d1a-8891-77f88794ce43")
	if err != nil {
		t.Fatal(err)
	}
	if id != "18d69307-e9af-4d1a-8891-77f88794ce43" {
		t.Fatalf("id = %q", id)
	}
}

func TestPandaVideoIDFromURL_RejectsNonPandaHost(t *testing.T) {
	if _, err := pandaVideoIDFromURL("https://iframe.mediadelivery.net/embed/383534/4e8da26a-754a-4ba7-9df7-02a0e8f2f396"); err == nil {
		t.Fatal("expected error for Bunny host")
	}
}

func TestPandaVideoIDFromURL_MissingVAndNonUUIDPath(t *testing.T) {
	if _, err := pandaVideoIDFromURL("https://player-vz-464ed8d8-7b7.tv.pandavideo.com.br/embed/"); err == nil {
		t.Fatal("expected error for missing v and non-UUID path")
	}
}

func TestPandaVideoIDFromURL_RejectsGarbage(t *testing.T) {
	if _, err := pandaVideoIDFromURL("not a url :: at all"); err == nil {
		t.Fatal("expected error for garbage input")
	}
}

func TestPandaVideoIDFromURL_PathBasedUUID(t *testing.T) {
	// Fallback: no `v` query param, but the last path segment is a UUID.
	id, err := pandaVideoIDFromURL("https://player-vz-464ed8d8-7b7.tv.pandavideo.com.br/embed/18d69307-e9af-4d1a-8891-77f88794ce43")
	if err != nil {
		t.Fatal(err)
	}
	if id != "18d69307-e9af-4d1a-8891-77f88794ce43" {
		t.Fatalf("id = %q", id)
	}
}

func TestIsPandaURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want bool
	}{
		{"embed shape", "https://player-vz-464ed8d8-7b7.tv.pandavideo.com.br/embed/?v=abc", true},
		{"bunny url", "https://iframe.mediadelivery.net/embed/1/2", false},
		{"empty string", "", false},
		{"pandavideo only in query value", "https://iframe.mediadelivery.net/embed/1/2?next=pandavideo.com", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isPandaURL(c.url); got != c.want {
				t.Fatalf("isPandaURL(%q) = %v, want %v", c.url, got, c.want)
			}
		})
	}
}

func TestPickPandaTrack_PrefersPortuguese(t *testing.T) {
	// pt-BR listed after en — must still be picked.
	tracks := []pandaSubtitleTrack{
		{SrcLang: "en"},
		{SrcLang: "pt-BR"},
	}
	got, err := pickPandaTrack(tracks)
	if err != nil {
		t.Fatal(err)
	}
	if got.SrcLang != "pt-BR" {
		t.Fatalf("SrcLang = %q, want pt-BR", got.SrcLang)
	}
}

func TestPickPandaTrack_PrefersPortugueseRegardlessOfOrder(t *testing.T) {
	tracks := []pandaSubtitleTrack{
		{SrcLang: "pt-BR"},
		{SrcLang: "en"},
	}
	got, err := pickPandaTrack(tracks)
	if err != nil {
		t.Fatal(err)
	}
	if got.SrcLang != "pt-BR" {
		t.Fatalf("SrcLang = %q, want pt-BR", got.SrcLang)
	}
}

func TestPickPandaTrack_EnglishOnly(t *testing.T) {
	tracks := []pandaSubtitleTrack{{SrcLang: "en"}}
	got, err := pickPandaTrack(tracks)
	if err != nil {
		t.Fatal(err)
	}
	if got.SrcLang != "en" {
		t.Fatalf("SrcLang = %q, want en", got.SrcLang)
	}
}

func TestPickPandaTrack_EmptyListErrorsWithDashboardHint(t *testing.T) {
	_, err := pickPandaTrack(nil)
	if err == nil || !strings.Contains(err.Error(), "dashboard") {
		t.Fatalf("expected error mentioning dashboard, got %v", err)
	}
}

func TestParseVTT_HeaderAndTwoCues(t *testing.T) {
	vtt := "WEBVTT\n\n" +
		"00:00:00.000 --> 00:00:03.000\n" +
		"Olá, bem-vindo ao vídeo.\n\n" +
		"00:00:03.000 --> 00:00:06.500\n" +
		"Hoje vamos aprender Go.\n"
	segs, err := parseVTT(vtt)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 2 {
		t.Fatalf("len(segs) = %d, want 2", len(segs))
	}
	if segs[0].Start != 0 || segs[0].End != 3 || segs[0].Text != "Olá, bem-vindo ao vídeo." {
		t.Fatalf("seg0 = %+v", segs[0])
	}
	if segs[1].Start != 3 || segs[1].End != 6.5 || segs[1].Text != "Hoje vamos aprender Go." {
		t.Fatalf("seg1 = %+v", segs[1])
	}
}

func TestParseVTT_CueIdentifierAndSettings(t *testing.T) {
	vtt := "WEBVTT\n\n" +
		"cue-1\n" +
		"00:00:00.000 --> 00:00:03.000 align:start position:0%\n" +
		"Texto com identificador.\n"
	segs, err := parseVTT(vtt)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 1 {
		t.Fatalf("len(segs) = %d, want 1", len(segs))
	}
	if segs[0].Text != "Texto com identificador." {
		t.Fatalf("text = %q", segs[0].Text)
	}
}

func TestParseVTT_ShortTimestamps(t *testing.T) {
	vtt := "WEBVTT\n\n" +
		"00:00.000 --> 00:03.500\n" +
		"Curto.\n"
	segs, err := parseVTT(vtt)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 1 {
		t.Fatalf("len(segs) = %d, want 1", len(segs))
	}
	if segs[0].Start != 0 || segs[0].End != 3.5 {
		t.Fatalf("seg = %+v", segs[0])
	}
}

func TestParseVTT_MultiLineCueTextJoined(t *testing.T) {
	vtt := "WEBVTT\n\n" +
		"00:00:00.000 --> 00:00:03.000\n" +
		"Primeira linha\n" +
		"segunda linha\n"
	segs, err := parseVTT(vtt)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 1 {
		t.Fatalf("len(segs) = %d, want 1", len(segs))
	}
	if segs[0].Text != "Primeira linha segunda linha" {
		t.Fatalf("text = %q", segs[0].Text)
	}
}

func TestParseVTT_SkipsNoteBlock(t *testing.T) {
	vtt := "WEBVTT\n\n" +
		"NOTE this is a comment\nspanning lines\n\n" +
		"00:00:00.000 --> 00:00:03.000\n" +
		"Depois da nota.\n"
	segs, err := parseVTT(vtt)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 1 {
		t.Fatalf("len(segs) = %d, want 1", len(segs))
	}
	if segs[0].Text != "Depois da nota." {
		t.Fatalf("text = %q", segs[0].Text)
	}
}

func TestParseVTT_StripsInlineTags(t *testing.T) {
	vtt := "WEBVTT\n\n" +
		"00:00:00.000 --> 00:00:03.000\n" +
		"<v Maria>Olá <c>mundo</c> <00:00:01.000>tudo bem<i>?</i>\n"
	segs, err := parseVTT(vtt)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 1 {
		t.Fatalf("len(segs) = %d, want 1", len(segs))
	}
	want := "Olá mundo tudo bem?"
	if segs[0].Text != want {
		t.Fatalf("text = %q, want %q", segs[0].Text, want)
	}
}

func TestParseVTT_TolerantOfBOM(t *testing.T) {
	vtt := "\uFEFFWEBVTT\n\n" +
		"00:00:00.000 --> 00:00:03.000\n" +
		"Com BOM.\n"
	segs, err := parseVTT(vtt)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 1 || segs[0].Text != "Com BOM." {
		t.Fatalf("segs = %+v", segs)
	}
}

func TestParseVTT_EmptyOrGarbageErrors(t *testing.T) {
	if _, err := parseVTT(""); err == nil {
		t.Fatal("expected error for empty input")
	}
	if _, err := parseVTT("this is not a vtt file at all"); err == nil {
		t.Fatal("expected error for garbage input")
	}
}

func TestParseVTT_PandaTimestampMapHeader(t *testing.T) {
	// Real Panda VTT bodies carry an X-TIMESTAMP-MAP metadata line right
	// after the WEBVTT header; it must be skipped, not parsed as a cue.
	vtt := "WEBVTT\nX-TIMESTAMP-MAP=LOCAL:00:00:00.000,MPEGTS:144540\n\n" +
		"00:00:00.320 --> 00:00:03.920\n" +
		"Seja muito bemvindo.\n"
	segs, err := parseVTT(vtt)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 1 {
		t.Fatalf("len(segs) = %d, want 1", len(segs))
	}
	if segs[0].Start != 0.32 || segs[0].End != 3.92 || segs[0].Text != "Seja muito bemvindo." {
		t.Fatalf("seg = %+v", segs[0])
	}
}

// newFakePandaVideosServer serves GET /videos over the given pages of
// items, tracking how many /videos requests arrived (per page and total).
func newFakePandaVideosServer(t *testing.T, pages [][]pandaVideoListItem, hits *[]int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/videos" {
			t.Fatalf("unexpected panda path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		page, err := strconv.Atoi(r.URL.Query().Get("page"))
		if err != nil || page < 1 || page > len(pages) {
			t.Fatalf("unexpected page param: %q", r.URL.Query().Get("page"))
		}
		*hits = append(*hits, page)

		total := 0
		for _, p := range pages {
			total += len(p)
		}
		resp := pandaVideoListResponse{Videos: pages[page-1], Pages: len(pages), Total: total}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestResolvePandaInternalID_HappyPathAndCache(t *testing.T) {
	var hits []int
	server := newFakePandaVideosServer(t, [][]pandaVideoListItem{
		{{ID: "internal-1", ExternalID: "external-1"}},
	}, &hits)
	defer server.Close()

	f := &Feature{pandaBaseURL: server.URL, pandaAPIKey: "test-key", httpClient: server.Client()}

	id, err := f.resolvePandaInternalID(context.Background(), "external-1")
	if err != nil {
		t.Fatal(err)
	}
	if id != "internal-1" {
		t.Fatalf("id = %q, want internal-1", id)
	}
	if len(hits) != 1 {
		t.Fatalf("scan requests after first resolve = %d, want 1", len(hits))
	}

	// Second call must be served from the cache — no new HTTP requests.
	id, err = f.resolvePandaInternalID(context.Background(), "external-1")
	if err != nil {
		t.Fatal(err)
	}
	if id != "internal-1" {
		t.Fatalf("cached id = %q, want internal-1", id)
	}
	if len(hits) != 1 {
		t.Fatalf("scan requests after cached resolve = %d, want 1", len(hits))
	}
}

func TestResolvePandaInternalID_InternalIDPassthrough(t *testing.T) {
	var hits []int
	server := newFakePandaVideosServer(t, [][]pandaVideoListItem{
		{{ID: "internal-1", ExternalID: "external-1"}},
	}, &hits)
	defer server.Close()

	f := &Feature{pandaBaseURL: server.URL, pandaAPIKey: "test-key", httpClient: server.Client()}

	// A URL that already carries the internal id resolves to itself.
	id, err := f.resolvePandaInternalID(context.Background(), "internal-1")
	if err != nil {
		t.Fatal(err)
	}
	if id != "internal-1" {
		t.Fatalf("id = %q, want internal-1", id)
	}
}

func TestResolvePandaInternalID_Pagination(t *testing.T) {
	var hits []int
	server := newFakePandaVideosServer(t, [][]pandaVideoListItem{
		{{ID: "internal-1", ExternalID: "external-1"}},
		{{ID: "internal-2", ExternalID: "external-2"}},
	}, &hits)
	defer server.Close()

	f := &Feature{pandaBaseURL: server.URL, pandaAPIKey: "test-key", httpClient: server.Client()}

	// Target lives on page 2 — the scan must walk both pages.
	id, err := f.resolvePandaInternalID(context.Background(), "external-2")
	if err != nil {
		t.Fatal(err)
	}
	if id != "internal-2" {
		t.Fatalf("id = %q, want internal-2", id)
	}
	if len(hits) != 2 || hits[0] != 1 || hits[1] != 2 {
		t.Fatalf("pages requested = %v, want [1 2]", hits)
	}
}

func TestResolvePandaInternalID_NotFoundThenRescan(t *testing.T) {
	var hits []int
	server := newFakePandaVideosServer(t, [][]pandaVideoListItem{
		{{ID: "internal-1", ExternalID: "external-1"}},
	}, &hits)
	defer server.Close()

	f := &Feature{pandaBaseURL: server.URL, pandaAPIKey: "test-key", httpClient: server.Client()}

	_, err := f.resolvePandaInternalID(context.Background(), "external-missing")
	if err == nil || !strings.Contains(err.Error(), "not found in Panda account") {
		t.Fatalf("expected not-found error, got %v", err)
	}
	scansAfterMiss := len(hits)
	if scansAfterMiss == 0 {
		t.Fatal("expected at least one /videos request on first miss")
	}

	// A later call for another unknown id must rescan (cache miss → new
	// requests), so newly-uploaded videos get picked up.
	_, err = f.resolvePandaInternalID(context.Background(), "external-also-missing")
	if err == nil || !strings.Contains(err.Error(), "not found in Panda account") {
		t.Fatalf("expected not-found error, got %v", err)
	}
	if len(hits) <= scansAfterMiss {
		t.Fatalf("expected a rescan for a new id: requests before=%d after=%d", scansAfterMiss, len(hits))
	}
}

func TestResolvePandaInternalID_PropagatesHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server exploded", http.StatusInternalServerError)
	}))
	defer server.Close()

	f := &Feature{pandaBaseURL: server.URL, pandaAPIKey: "test-key", httpClient: server.Client()}
	_, err := f.resolvePandaInternalID(context.Background(), "external-1")
	if err == nil || !strings.Contains(err.Error(), "status=500") {
		t.Fatalf("expected status=500 error, got %v", err)
	}
}

func TestFetchPandaSubtitleTracks_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "test-key" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if !strings.HasSuffix(r.URL.Path, "/subtitles/vid-1") {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"subtitles":[
			{"srclang":"pt-BR","label":"Português (BR)","hidden":false,"is_uploaded":true},
			{"srclang":"en","label":"English","hidden":false,"transcription":true}
		]}`))
	}))
	defer server.Close()

	f := &Feature{pandaBaseURL: server.URL, pandaAPIKey: "test-key", httpClient: server.Client()}
	tracks, err := f.fetchPandaSubtitleTracks(context.Background(), "vid-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 || tracks[0].SrcLang != "pt-BR" || tracks[1].SrcLang != "en" {
		t.Fatalf("tracks = %+v", tracks)
	}
}

func TestFetchPandaSubtitleTracks_PropagatesHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	f := &Feature{pandaBaseURL: server.URL, pandaAPIKey: "bad-key", httpClient: server.Client()}
	_, err := f.fetchPandaSubtitleTracks(context.Background(), "vid-1")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got %v", err)
	}
}

func TestFetchPandaVTT_HappyPath(t *testing.T) {
	body := "WEBVTT\n\n00:00:00.000 --> 00:00:03.000\nOlá, bem-vindo ao vídeo.\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "test-key" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if !strings.HasSuffix(r.URL.Path, "/subtitles/vid-1/pt-BR") {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	f := &Feature{pandaBaseURL: server.URL, pandaAPIKey: "test-key", httpClient: server.Client()}
	got, err := f.fetchPandaVTT(context.Background(), "vid-1", "pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	if got != body {
		t.Fatalf("body = %q, want %q", got, body)
	}
}

func TestFetchPandaVTT_NoTrackForLanguage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no such track", http.StatusBadRequest)
	}))
	defer server.Close()

	f := &Feature{pandaBaseURL: server.URL, pandaAPIKey: "test-key", httpClient: server.Client()}
	_, err := f.fetchPandaVTT(context.Background(), "vid-1", "es")
	if err == nil || !strings.Contains(err.Error(), "no subtitle") {
		t.Fatalf("expected 'no subtitle' error, got %v", err)
	}
}

func TestFetchPandaVTT_PropagatesOtherHTTPErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server exploded", http.StatusInternalServerError)
	}))
	defer server.Close()

	f := &Feature{pandaBaseURL: server.URL, pandaAPIKey: "test-key", httpClient: server.Client()}
	_, err := f.fetchPandaVTT(context.Background(), "vid-1", "pt-BR")
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got %v", err)
	}
}
