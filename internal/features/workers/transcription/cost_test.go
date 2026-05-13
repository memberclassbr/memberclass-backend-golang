package transcription

import "testing"

func TestWhisperCostCents(t *testing.T) {
	tests := []struct {
		name    string
		seconds float64
		want    int
	}{
		{"zero", 0, 0},
		{"negative is zero", -10, 0},
		// 60s = 1 min = 0.6¢ → ceil to 1¢
		{"one minute", 60, 1},
		// 300s = 5 min = 3¢
		{"five minutes", 300, 3},
		// 90s = 1.5 min = 0.9¢ → ceil to 1¢
		{"90 seconds rounds up", 90, 1},
		// 30 min = 18¢
		{"thirty minutes", 1800, 18},
		// 1 hour = 36¢
		{"one hour", 3600, 36},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := whisperCostCents(tt.seconds); got != tt.want {
				t.Fatalf("whisperCostCents(%v) = %d, want %d", tt.seconds, got, tt.want)
			}
		})
	}
}

func TestEmbedCostCents(t *testing.T) {
	tests := []struct {
		name   string
		tokens int
		want   int
	}{
		{"zero", 0, 0},
		{"negative is zero", -100, 0},
		// 1M tokens = 2¢
		{"one million", 1_000_000, 2},
		// 500k tokens = 1¢
		{"half million", 500_000, 1},
		// 100k tokens = 0.2¢ → ceil to 1¢
		{"100k rounds up", 100_000, 1},
		// 1 token = round up to 1¢
		{"single token rounds up", 1, 1},
		// 5M tokens = 10¢
		{"five million", 5_000_000, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := embedCostCents(tt.tokens); got != tt.want {
				t.Fatalf("embedCostCents(%d) = %d, want %d", tt.tokens, got, tt.want)
			}
		})
	}
}
