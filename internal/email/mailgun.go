package email

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Mailgun regions an operator can pick.
const (
	MailgunRegionUS = "us"
	MailgunRegionEU = "eu"
)

type mailgunSender struct {
	cfg     APIConfig
	baseURL string
}

func init() {
	Register(BackendMailgun, func(cfg Config) (Sender, error) {
		api, err := requireAPI(cfg, "mailgun")
		if err != nil {
			return nil, err
		}
		if api.Domain == "" {
			return nil, fmt.Errorf("email: mailgun backend selected with no sending domain")
		}
		return mailgunSender{cfg: *api}, nil
	})
}

func (s mailgunSender) Send(ctx context.Context, to, subject, body string) error {
	_, err := s.sendID(ctx, to, subject, body)
	return err
}

func (s mailgunSender) sendID(ctx context.Context, to, subject, body string) (string, error) {
	return s.sendMessageID(ctx, Message{To: to, Subject: subject, Text: body})
}

func (s mailgunSender) SendMessage(ctx context.Context, m Message) error {
	_, err := s.sendMessageID(ctx, m)
	return err
}

func (s mailgunSender) sendMessageID(ctx context.Context, m Message) (string, error) {
	if err := validateMessage(m); err != nil {
		return "", err
	}
	base := s.baseURL
	if base == "" {
		base = "https://api.mailgun.net"
		if s.cfg.Region == MailgunRegionEU {
			base = "https://api.eu.mailgun.net"
		}
	}
	form := url.Values{"from": {s.cfg.From}, "to": {m.To}, "subject": {m.Subject}, "text": {m.Text}}
	if m.HTML != "" {
		form.Set("html", m.HTML)
	}
	for k, v := range m.Headers {
		form.Set("h:"+k, v)
	}
	if s.cfg.ReplyTo != "" {
		form.Set("h:Reply-To", s.cfg.ReplyTo)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v3/"+url.PathEscape(s.cfg.Domain)+"/messages", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("email: build mailgun request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("api", s.cfg.APIKey)
	out, _, err := callAPI("mailgun", req)
	if err != nil {
		return "", err
	}
	var resp struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(out, &resp)
	return strings.Trim(resp.ID, "<>"), nil
}
