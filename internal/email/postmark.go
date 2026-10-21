package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const defaultPostmarkURL = "https://api.postmarkapp.com/email"

type postmarkSender struct {
	cfg    APIConfig
	apiURL string
}

func init() {
	Register(BackendPostmark, func(cfg Config) (Sender, error) {
		api, err := requireAPI(cfg, "postmark")
		if err != nil {
			return nil, err
		}
		return postmarkSender{cfg: *api}, nil
	})
}

func (s postmarkSender) Send(ctx context.Context, to, subject, body string) error {
	_, err := s.sendID(ctx, to, subject, body)
	return err
}

func (s postmarkSender) sendID(ctx context.Context, to, subject, body string) (string, error) {
	return s.sendMessageID(ctx, Message{To: to, Subject: subject, Text: body})
}

func (s postmarkSender) SendMessage(ctx context.Context, m Message) error {
	_, err := s.sendMessageID(ctx, m)
	return err
}

func (s postmarkSender) sendMessageID(ctx context.Context, m Message) (string, error) {
	if err := validateMessage(m); err != nil {
		return "", err
	}
	url := s.apiURL
	if url == "" {
		url = defaultPostmarkURL
	}
	msg := map[string]any{"From": s.cfg.From, "To": m.To, "Subject": m.Subject, "TextBody": m.Text, "ReplyTo": s.cfg.ReplyTo}
	if m.HTML != "" {
		msg["HtmlBody"] = m.HTML
	}
	if len(m.Headers) > 0 {
		hs := make([]map[string]string, 0, len(m.Headers))
		for k, v := range m.Headers {
			hs = append(hs, map[string]string{"Name": k, "Value": v})
		}
		msg["Headers"] = hs
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("email: encode postmark request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("email: build postmark request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Postmark-Server-Token", s.cfg.APIKey)
	out, _, err := callAPI("postmark", req)
	if err != nil {
		return "", err
	}
	var resp struct {
		MessageID string `json:"MessageID"`
	}
	_ = json.Unmarshal(out, &resp)
	return resp.MessageID, nil
}
