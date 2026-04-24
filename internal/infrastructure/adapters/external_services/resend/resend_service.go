// Package resend is a thin HTTP client for the Resend batch-email API.
//
// Only the batch endpoint (POST /emails/batch, up to 100 items per call) is
// exposed because the only caller today (the member_import slice) sends
// transactional emails in bulk. If a single-email sender is needed later,
// add a Send method that wraps POST /emails.
package resend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/ports"
)

const (
	defaultBaseURL = "https://api.resend.com"
	defaultTimeout = 30 * time.Second
	// MaxBatchSize is Resend's hard limit per batch call.
	MaxBatchSize = 100
)

// Email is a single entry in a batch send.
type Email struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html,omitempty"`
	Text    string   `json:"text,omitempty"`
}

// BatchResult mirrors what Resend returns: an array of IDs and optional error.
type BatchResult struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// Service sends transactional emails.
type Service interface {
	SendBatch(ctx context.Context, emails []Email) (*BatchResult, error)
}

type client struct {
	http    *http.Client
	apiKey  string
	baseURL string
	log     ports.Logger
}

// New builds a Service. Requires RESEND_API_KEY; otherwise the client fails
// fast on the first send (it does NOT fail at construction — the app should
// still boot so non-email slices keep working during a misconfiguration).
func New(log ports.Logger) Service {
	baseURL := os.Getenv("RESEND_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &client{
		http:    &http.Client{Timeout: defaultTimeout},
		apiKey:  os.Getenv("RESEND_API_KEY"),
		baseURL: baseURL,
		log:     log,
	}
}

func (c *client) SendBatch(ctx context.Context, emails []Email) (*BatchResult, error) {
	if c.apiKey == "" {
		return nil, errors.New("resend: RESEND_API_KEY is not set")
	}
	if len(emails) == 0 {
		return &BatchResult{}, nil
	}
	if len(emails) > MaxBatchSize {
		return nil, fmt.Errorf("resend: batch size %d exceeds max %d", len(emails), MaxBatchSize)
	}

	body, err := json.Marshal(emails)
	if err != nil {
		return nil, fmt.Errorf("resend: marshal batch: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/emails/batch", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("resend: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("resend: http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("resend: read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		c.log.Error("resend batch failed",
			"status", resp.StatusCode,
			"body", string(raw),
		)
		return nil, fmt.Errorf("resend: status %d: %s", resp.StatusCode, string(raw))
	}

	var result BatchResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("resend: decode response: %w", err)
	}
	return &result, nil
}
