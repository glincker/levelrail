package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
)

const defaultSendGridURL = "https://api.sendgrid.com/v3/mail/send"

type sendgridSender struct {
	cfg    APIConfig
	apiURL string
}

func init() {
	Register(BackendSendGrid, func(cfg Config) (Sender, error) {
		api, err := requireAPI(cfg, "sendgrid")
		if err != nil {
			return nil, err
		}
		return sendgridSender{cfg: *api}, nil
	})
}

type sendgridAddr struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

func (s sendgridSender) Send(ctx context.Context, to, subject, body string) error {
	_, err := s.sendID(ctx, to, subject, body)
	return err
}

func (s sendgridSender) sendID(ctx context.Context, to, subject, body string) (string, error) {
	return s.sendMessageID(ctx, Message{To: to, Subject: subject, Text: body})
}

func (s sendgridSender) SendMessage(ctx context.Context, m Message) error {
	_, err := s.sendMessageID(ctx, m)
	return err
}

func (s sendgridSender) sendMessageID(ctx context.Context, m Message) (string, error) {
	if err := validateMessage(m); err != nil {
		return "", err
	}
	url := s.apiURL
	if url == "" {
		url = defaultSendGridURL
	}
	from, err := mail.ParseAddress(s.cfg.From)
	if err != nil {
		return "", fmt.Errorf("email: invalid from address: %w", err)
	}
	content := []map[string]string{{"type": "text/plain", "value": m.Text}}
	if m.HTML != "" {
		content = append(content, map[string]string{"type": "text/html", "value": m.HTML})
	}
	msg := map[string]any{
		"personalizations": []map[string]any{{"to": []sendgridAddr{{Email: m.To}}}},
		"from":             sendgridAddr{Email: from.Address, Name: from.Name},
		"subject":          m.Subject,
		"content":          content,
	}
	if len(m.Headers) > 0 {
		msg["headers"] = m.Headers
	}
	if s.cfg.ReplyTo != "" {
		msg["reply_to"] = sendgridAddr{Email: s.cfg.ReplyTo}
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("email: encode sendgrid request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("email: build sendgrid request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	_, hdr, err := callAPI("sendgrid", req)
	if err != nil {
		return "", err
	}
	return hdr.Get("X-Message-Id"), nil
}
