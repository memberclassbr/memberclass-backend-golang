package transcription

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// defaultOpenAIBase is overridable per-Feature so tests can point at an
// httptest server.
const defaultOpenAIBase = "https://api.openai.com"

const (
	// embedModel: text-embedding-3-small supports the `dimensions` request
	// param (Matryoshka), so we ask the API to truncate to whatever width
	// the chunks.embedding column was declared with (see Feature.embedDims).
	// Mixing models or widths against the same column invalidates prior
	// rows — bring up a new column instead of overloading this one.
	embedModel = "text-embedding-3-small"

	// whisperModel: whisper-1 is the only Whisper variant exposed by the
	// OpenAI HTTP API today.
	whisperModel = "whisper-1"

	// defaultEmbedDims is the fallback when the runtime probe of
	// chunks.embedding fails (e.g. table absent on a fresh DB). It matches
	// the legacy Supabase column width.
	defaultEmbedDims = 768
)

// ---------- response types ----------

type embedding struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type embeddingsResponse struct {
	Data  []embedding `json:"data"`
	Usage usage       `json:"usage"`
	Model string      `json:"model"`
}

type whisperSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type whisperResponse struct {
	Text     string           `json:"text"`
	Language string           `json:"language"`
	Duration float64          `json:"duration"`
	Segments []whisperSegment `json:"segments"`
}

// ---------- client methods ----------

// embedBatch sends `inputs` to /v1/embeddings and returns one vector per
// input (positions matching `inputs`) and the total token count reported
// by the API (used for cost tracking).
func (f *Feature) embedBatch(ctx context.Context, inputs []string) ([][]float32, int, error) {
	dims := f.embedDims
	if dims <= 0 {
		dims = defaultEmbedDims
	}
	body, err := json.Marshal(map[string]any{
		"model":      embedModel,
		"input":      inputs,
		"dimensions": dims,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("marshal embed payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		f.openaiBaseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("build embed request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.openaiAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("openai embed http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("openai embed status=%d body=%s", resp.StatusCode, string(b))
	}

	var parsed embeddingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, 0, fmt.Errorf("openai embed decode: %w", err)
	}

	// Reassemble by `Index` rather than trusting positional order; the API
	// reliably echoes input order today but the field exists to support
	// out-of-order responses, so honor it.
	vecs := make([][]float32, len(inputs))
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= len(vecs) {
			return nil, 0, fmt.Errorf("openai embed: bad index %d (have %d inputs)", d.Index, len(inputs))
		}
		vecs[d.Index] = d.Embedding
	}
	for i, v := range vecs {
		if v == nil {
			return nil, 0, fmt.Errorf("openai embed: missing vector at index %d", i)
		}
	}

	return vecs, parsed.Usage.TotalTokens, nil
}

// transcribeAudio uploads `audio` (an MP3/M4A reader) to /v1/audio/transcriptions
// using whisper-1 in verbose_json mode (so we get segments with timestamps).
// The `filename` is required by OpenAI's multipart contract; the extension
// drives format auto-detection on their side.
func (f *Feature) transcribeAudio(ctx context.Context, audio io.Reader, filename string) (*whisperResponse, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("multipart create file part: %w", err)
	}
	if _, err := io.Copy(fw, audio); err != nil {
		return nil, fmt.Errorf("multipart copy audio: %w", err)
	}
	if err := mw.WriteField("model", whisperModel); err != nil {
		return nil, err
	}
	if err := mw.WriteField("response_format", "verbose_json"); err != nil {
		return nil, err
	}
	// Force Portuguese; the legacy lessons are all pt-BR and Whisper's
	// language guess on short / accented audio occasionally drifts to ES.
	if err := mw.WriteField("language", "pt"); err != nil {
		return nil, err
	}
	if err := mw.WriteField("timestamp_granularities[]", "segment"); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("multipart close: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		f.openaiBaseURL+"/v1/audio/transcriptions", &buf)
	if err != nil {
		return nil, fmt.Errorf("build whisper request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.openaiAPIKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai whisper http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai whisper status=%d body=%s", resp.StatusCode, string(b))
	}

	var parsed whisperResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("openai whisper decode: %w", err)
	}
	return &parsed, nil
}
