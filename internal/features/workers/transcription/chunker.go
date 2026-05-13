package transcription

import "strings"

// chunk is the unit we persist to the chunks table. Order matches the
// transcript order within a video; start/end carry the Whisper timestamps
// for the FIRST and LAST segments that fed this chunk (so RAG citations
// can deep-link into the video timeline).
type chunk struct {
	Order     int
	Text      string
	StartTime float64
	EndTime   float64
	Tokens    int
}

// splitIntoChunks groups Whisper segments into chunks of approximately
// `maxTokens` with `overlap` tokens of carry-over between consecutive
// chunks. Splitting respects segment boundaries — we never cut inside a
// Whisper segment, which keeps the start/end timestamps meaningful.
//
// Token count uses a cheap approximation: `wordCount * 1.3`. cl100k_base
// runs ~1.3 tokens/word for Portuguese; if the over-estimate proves
// problematic for cost we can swap in tiktoken-go.
//
// Behavior on degenerate input:
//   - len(segments) == 0 returns nil
//   - a single segment that exceeds maxTokens still produces one chunk
//     (we don't subdivide segments)
//   - overlap >= maxTokens is treated as overlap = maxTokens - 1 so we
//     always advance
func splitIntoChunks(segments []whisperSegment, maxTokens, overlap int) []chunk {
	if len(segments) == 0 {
		return nil
	}
	if maxTokens < 1 {
		maxTokens = 1
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= maxTokens {
		overlap = maxTokens - 1
	}

	// Pre-compute per-segment token counts so the backtrack loop is O(1)
	// per segment.
	segTokens := make([]int, len(segments))
	for i, s := range segments {
		segTokens[i] = approxTokens(s.Text)
	}

	var out []chunk
	var cur chunk
	cur.StartTime = segments[0].Start
	curFirst := 0 // index of first segment included in `cur`

	flush := func() {
		if cur.Tokens == 0 {
			return
		}
		cur.Order = len(out)
		cur.Text = strings.TrimSpace(cur.Text)
		out = append(out, cur)
	}

	for i, seg := range segments {
		tks := segTokens[i]
		// If adding this segment would overflow AND we already have
		// something to flush, emit the current chunk and start a new
		// one that overlaps with the tail of the previous.
		if cur.Tokens > 0 && cur.Tokens+tks > maxTokens {
			flush()

			// Walk back from i to gather `overlap` tokens worth of
			// segments to replay into the next chunk.
			carry := i
			carried := 0
			for j := i - 1; j >= curFirst && carried < overlap; j-- {
				carried += segTokens[j]
				carry = j
			}

			cur = chunk{StartTime: segments[carry].Start}
			for j := carry; j < i; j++ {
				cur.Text += segments[j].Text + " "
				cur.Tokens += segTokens[j]
				cur.EndTime = segments[j].End
			}
			curFirst = carry
		}
		cur.Text += seg.Text + " "
		cur.Tokens += tks
		cur.EndTime = seg.End
		if cur.Order == 0 && cur.Tokens == tks {
			// First segment of this chunk — remember its index for the
			// next overlap-backtrack.
			curFirst = i
		}
	}
	flush()
	return out
}

// approxTokens estimates cl100k_base tokens for a string using word count
// scaled by 1.3 (empirical average for Latin-script text). Deliberately
// cheap; do NOT use this for billing — token counts in token_usage come
// from the OpenAI response.
func approxTokens(s string) int {
	words := len(strings.Fields(s))
	return (words * 13) / 10
}
