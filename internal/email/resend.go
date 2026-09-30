package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// defaultResendAPIURL is Resend's own fixed endpoint; resendSender's
// apiURL field defaults to this and is only ever overridden by this
// package's own tests.
const defaultResendAPIURL = "https://api.resend.com/emails"

// ResendConfig is a Resend sender's connection details.
type ResendConfig struct {
	APIKey string
	From   string
}

// resendSender sends via Resend's HTTP API: a single POST with a bearer
// token, the simplest of the backends this package supports, no SDK
// needed for one endpoint.
type resendSender struct {
	cfg    ResendConfig
	apiURL string // "" uses defaultResendAPIURL
}

type resendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

func (s resendSender) Send(ctx context.Context, to, subject, body string) error {
	apiURL := s.apiURL
	if apiURL == "" {
		apiURL = defaultResendAPIURL
	}

	payload, err := json.Marshal(resendEmailRequest{From: s.cfg.From, To: []string{to}, Subject: subject, Text: body})
	if err != nil {
		return fmt.Errorf("email: encode resend request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("email: build resend request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("email: send via resend: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("email: resend returned status %d: %s", resp.StatusCode, string(msg))
	}
	return nil
}
