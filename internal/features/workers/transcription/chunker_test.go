package transcription

import (
	"strings"
	"testing"
)

func TestSplitIntoChunks_EmptyInput(t *testing.T) {
	if got := splitIntoChunks(nil, 500, 50); got != nil {
		t.Fatalf("expected nil for empty input, got %+v", got)
	}
}

func TestSplitIntoChunks_SingleSegmentFitsInOne(t *testing.T) {
	segs := []whisperSegment{
		{Start: 0, End: 5, Text: "Olá mundo. Esta é uma aula de Go."},
	}
	chunks := splitIntoChunks(segs, 500, 50)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	if chunks[0].StartTime != 0 || chunks[0].EndTime != 5 {
		t.Fatalf("bad timestamps: %+v", chunks[0])
	}
	if chunks[0].Order != 0 {
		t.Fatalf("order = %d", chunks[0].Order)
	}
}

func TestSplitIntoChunks_OverflowProducesMultiple(t *testing.T) {
	// 30 segments × ~65 tokens each ≈ 1950 tokens → ~4 chunks of 500
	body := strings.Repeat("palavra ", 50) // ~50 words → ~65 tokens
	segs := make([]whisperSegment, 30)
	for i := range segs {
		segs[i] = whisperSegment{
			Start: float64(i * 2),
			End:   float64(i*2 + 2),
			Text:  body,
		}
	}
	chunks := splitIntoChunks(segs, 500, 50)
	if len(chunks) < 3 {
		t.Fatalf("expected >=3 chunks, got %d", len(chunks))
	}
	// Order monotonically increasing
	for i := 1; i < len(chunks); i++ {
		if chunks[i].Order != chunks[i-1].Order+1 {
			t.Fatalf("non-monotonic order at %d: %+v", i, chunks)
		}
	}
	// Chunks should not be empty
	for i, c := range chunks {
		if c.Text == "" {
			t.Fatalf("chunk %d empty", i)
		}
		if c.EndTime < c.StartTime {
			t.Fatalf("chunk %d has inverted timestamps", i)
		}
	}
}

func TestSplitIntoChunks_OverlapCoversTailOfPrevious(t *testing.T) {
	segs := []whisperSegment{
		{Start: 0, End: 1, Text: strings.Repeat("foo ", 50)},   // ~65 tokens
		{Start: 1, End: 2, Text: strings.Repeat("bar ", 50)},   // ~65 tokens
		{Start: 2, End: 3, Text: strings.Repeat("baz ", 50)},   // ~65 tokens
		{Start: 3, End: 4, Text: strings.Repeat("qux ", 50)},   // ~65 tokens
	}
	chunks := splitIntoChunks(segs, 100, 30)
	if len(chunks) < 2 {
		t.Fatalf("expected multi-chunk split, got %d", len(chunks))
	}
	// The second chunk should include some content from the tail of the
	// first (overlap >= 30 tokens means at least one prior segment carries).
	last := chunks[0].Text
	next := chunks[1].Text
	if !strings.Contains(next, lastWord(last)) {
		t.Fatalf("expected chunk[1] to overlap with tail of chunk[0]\nchunk[0]: %q\nchunk[1]: %q", last, next)
	}
}

func TestSplitIntoChunks_LongSingleSegmentIsKept(t *testing.T) {
	// One segment larger than maxTokens — we should still emit one chunk
	// rather than dropping data.
	segs := []whisperSegment{
		{Start: 0, End: 60, Text: strings.Repeat("foo ", 1000)}, // ~1300 tokens
	}
	chunks := splitIntoChunks(segs, 500, 50)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1 (oversize segment kept whole)", len(chunks))
	}
}

func TestSplitIntoChunks_OverlapAtOrAboveMaxDoesNotLoop(t *testing.T) {
	// Pathological config: overlap >= maxTokens. Should clamp internally
	// so the function still terminates and produces chunks.
	segs := []whisperSegment{
		{Start: 0, End: 1, Text: strings.Repeat("foo ", 80)},
		{Start: 1, End: 2, Text: strings.Repeat("bar ", 80)},
	}
	chunks := splitIntoChunks(segs, 50, 50) // overlap == maxTokens
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk, got 0")
	}
}

func TestApproxTokens(t *testing.T) {
	// 10 words ≈ 13 tokens
	if got := approxTokens("um dois três quatro cinco seis sete oito nove dez"); got != 13 {
		t.Fatalf("approxTokens 10 words = %d, want 13", got)
	}
	if got := approxTokens(""); got != 0 {
		t.Fatalf("empty = %d, want 0", got)
	}
}

// lastWord returns the last whitespace-separated word in s (helper for
// overlap assertions; chunks always end with a trailing space-stripped tail).
func lastWord(s string) string {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}
