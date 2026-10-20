package email

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// Message is an email with an HTML alternative and extra headers such as
// List-Unsubscribe.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
	Headers map[string]string
}

// RichSender is implemented by senders that can deliver Message with its
// HTML part and headers intact.
type RichSender interface {
	SendMessage(ctx context.Context, m Message) error
}

// SendMessage delivers m through s. A Sender without HTML support gets the
// plain-text part through Send, so mail never silently disappears.
func SendMessage(ctx context.Context, s Sender, m Message) error {
	if rs, ok := s.(RichSender); ok {
		return rs.SendMessage(ctx, m)
	}
	return s.Send(ctx, m.To, m.Subject, m.Text)
}

func validateMessage(m Message) error {
	if err := rejectHeaderInjection(m.To, m.Subject); err != nil {
		return err
	}
	for k, v := range m.Headers {
		if strings.ContainsAny(k+v, "\r\n") || strings.ContainsAny(k, ": ") {
			return fmt.Errorf("email: header %q is not valid", k)
		}
	}
	return nil
}

// SendMessage implements RichSender through the current settings.
func (d *DynamicSender) SendMessage(ctx context.Context, m Message) error {
	if err := validateMessage(m); err != nil {
		return err
	}
	cfg, err := d.load(ctx)
	if err != nil {
		return fmt.Errorf("email: load config: %w", err)
	}
	sender, err := NewSender(cfg)
	if err != nil {
		return err
	}
	return SendMessage(ctx, sender, m)
}

func (s smtpSender) SendMessage(_ context.Context, m Message) error {
	rcpt, msg, err := buildSMTPMultipart(s.cfg.From, m)
	if err != nil {
		return err
	}
	var auth smtp.Auth
	if s.cfg.Username != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}
	if err := smtp.SendMail(s.cfg.Addr, auth, s.cfg.From, []string{rcpt}, msg); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}

func buildSMTPMultipart(from string, m Message) (string, []byte, error) {
	if err := validateMessage(m); err != nil {
		return "", nil, err
	}
	rcpt, err := mail.ParseAddress(m.To)
	if err != nil {
		return "", nil, fmt.Errorf("email: invalid to address: %w", err)
	}
	sender, err := mail.ParseAddress(from)
	if err != nil {
		return "", nil, fmt.Errorf("email: invalid from address: %w", err)
	}
	idBytes := make([]byte, 8)
	_, _ = rand.Read(idBytes)

	var b bytes.Buffer
	b.WriteString("To: " + (&mail.Address{Address: rcpt.Address}).String() + "\r\n")
	b.WriteString("From: " + sender.String() + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", m.Subject) + "\r\n")
	b.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("Message-ID: <" + hex.EncodeToString(idBytes) + "@mail.invalid>\r\n")
	keys := make([]string, 0, len(m.Headers))
	for k := range m.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(k + ": " + m.Headers[k] + "\r\n")
	}
	b.WriteString("MIME-Version: 1.0\r\n")

	mw := multipart.NewWriter(&b)
	b.WriteString("Content-Type: multipart/alternative; boundary=" + mw.Boundary() + "\r\n\r\n")
	for _, part := range []struct{ ctype, body string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
		if part.body == "" {
			continue
		}
		ph := textproto.MIMEHeader{}
		ph.Set("Content-Type", part.ctype+"; charset=utf-8")
		ph.Set("Content-Transfer-Encoding", "quoted-printable")
		w, err := mw.CreatePart(ph)
		if err != nil {
			return "", nil, fmt.Errorf("email: build multipart: %w", err)
		}
		qp := quotedprintable.NewWriter(w)
		_, _ = qp.Write([]byte(part.body))
		_ = qp.Close()
	}
	_ = mw.Close()
	return rcpt.Address, b.Bytes(), nil
}

type resendRichRequest struct {
	From    string            `json:"from"`
	To      []string          `json:"to"`
	Subject string            `json:"subject"`
	Text    string            `json:"text"`
	HTML    string            `json:"html,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (s resendSender) SendMessage(ctx context.Context, m Message) error {
	if err := validateMessage(m); err != nil {
		return err
	}
	apiURL := s.apiURL
	if apiURL == "" {
		apiURL = defaultResendAPIURL
	}
	payload, err := json.Marshal(resendRichRequest{From: s.cfg.From, To: []string{m.To}, Subject: m.Subject, Text: m.Text, HTML: m.HTML, Headers: m.Headers})
	if err != nil {
		return fmt.Errorf("email: encode resend request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("email: build resend request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("email: send via resend: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("email: resend returned status %d: %s", resp.StatusCode, string(msg))
	}
	return nil
}

func (s sesSender) SendMessage(ctx context.Context, m Message) error {
	if err := validateMessage(m); err != nil {
		return err
	}
	client, err := s.client(ctx)
	if err != nil {
		return fmt.Errorf("email: build ses client: %w", err)
	}
	body := &types.Body{Text: &types.Content{Data: aws.String(m.Text), Charset: aws.String("UTF-8")}}
	if m.HTML != "" {
		body.Html = &types.Content{Data: aws.String(m.HTML), Charset: aws.String("UTF-8")}
	}
	msg := &types.Message{Subject: &types.Content{Data: aws.String(m.Subject), Charset: aws.String("UTF-8")}, Body: body}
	for k, v := range m.Headers {
		msg.Headers = append(msg.Headers, types.MessageHeader{Name: aws.String(k), Value: aws.String(v)})
	}
	input := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(s.cfg.From),
		Destination:      &types.Destination{ToAddresses: []string{m.To}},
		Content:          &types.EmailContent{Simple: msg},
	}
	if _, err := client.SendEmail(ctx, input); err != nil {
		return fmt.Errorf("email: send via ses: %w", err)
	}
	return nil
}
