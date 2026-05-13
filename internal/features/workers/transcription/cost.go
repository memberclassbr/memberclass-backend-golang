package transcription

import "math"

// OpenAI pricing as of 2026-05. Stored as cents-per-unit so the rounding
// happens on the unit we report (integer cents). Update both constants and
// the bundled tests when prices change.
const (
	// whisperCentsPerMinute: whisper-1 is billed at $0.006/min = 0.6¢/min.
	whisperCentsPerMinute = 0.6

	// embedCentsPer1MTokens: text-embedding-3-small is $0.02 / 1M tokens
	// = 2¢ / 100k tokens. Stored at 1M-token granularity to avoid float
	// drift on the per-token math.
	embedCentsPer1MTokens = 2.0
)

// whisperCostCents returns the cost in integer cents for transcribing
// `durationSeconds` of audio. Rounds UP so we never under-report the bill.
func whisperCostCents(durationSeconds float64) int {
	if durationSeconds <= 0 {
		return 0
	}
	cents := (durationSeconds / 60.0) * whisperCentsPerMinute
	return int(math.Ceil(cents))
}

// embedCostCents returns the cost in integer cents for embedding `tokens`
// tokens with text-embedding-3-small. Rounds UP.
func embedCostCents(tokens int) int {
	if tokens <= 0 {
		return 0
	}
	cents := float64(tokens) * (embedCentsPer1MTokens / 1_000_000.0)
	return int(math.Ceil(cents))
}
