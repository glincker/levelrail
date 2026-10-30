package email

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

const smtpDialTimeout = 15 * time.Second

type smtpSender struct {
	cfg SMTPConfig
}

func (s smtpSender) Send(ctx context.Context, to, subject, body string) error {
	_, err := s.sendID(ctx, to, subject, body)
	return err
}

func (s smtpSender) sendID(ctx context.Context, to, subject, body string) (string, error) {
	rcpt, msg, id, err := buildSMTPMessageFull(s.cfg.From, s.cfg.ReplyTo, to, subject, body)
	if err != nil {
		return "", err
	}
	envelope := s.cfg.From
	if s.cfg.EnvelopeFrom != "" {
		envelope = s.cfg.EnvelopeFrom
	}
	envAddr, err := mail.ParseAddress(envelope)
	if err != nil {
		return "", fmt.Errorf("email: invalid envelope sender: %w", err)
	}
	if err := s.deliver(ctx, envAddr.Address, rcpt, msg); err != nil {
		return "", fmt.Errorf("send email: %w", err)
	}
	return id, nil
}

func (s smtpSender) deliver(ctx context.Context, envelopeFrom, rcpt string, msg []byte) error {
	host, port, err := net.SplitHostPort(s.cfg.Addr)
	if err != nil {
		return err
	}
	mode := s.cfg.TLSMode
	if mode == TLSAuto && port == "465" {
		mode = TLSImplicit
	}
	tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: s.cfg.SkipVerify} //nolint:gosec // operator opt-out, off by default
	if s.cfg.PinSHA256 != "" {
		tlsCfg = pinnedTLSConfig(host, s.cfg.PinSHA256)
	}

	dialer := &net.Dialer{Timeout: smtpDialTimeout}
	var conn net.Conn
	if mode == TLSImplicit {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsCfg}).DialContext(ctx, "tcp", s.cfg.Addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", s.cfg.Addr)
	}
	if err != nil {
		return preSend(err)
	}
	deadline := time.Now().Add(60 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return preSend(err)
	}
	defer func() { _ = c.Close() }()

	if mode != TLSImplicit && mode != TLSNone {
		offered, _ := c.Extension("STARTTLS")
		switch {
		case offered:
			if err := c.StartTLS(tlsCfg); err != nil {
				return preSend(fmt.Errorf("starttls: %w", err))
			}
		case mode == TLSStartTLS:
			return preSend(errors.New("server does not offer STARTTLS"))
		}
	}
	if s.cfg.Username != "" {
		auth := smtp.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, host))
		if mode == TLSNone {
			auth = plainAuthAnyTransport{user: s.cfg.Username, pass: s.cfg.Password}
		}
		if err := c.Auth(auth); err != nil {
			return preSend(err)
		}
	}
	if err := c.Mail(envelopeFrom); err != nil {
		return preSend(err)
	}
	if err := c.Rcpt(rcpt); err != nil {
		return preSend(err)
	}
	w, err := c.Data()
	if err != nil {
		return preSend(err)
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// plainAuthAnyTransport is PLAIN auth for the explicit "none" TLS mode, where the operator chose plaintext.
type plainAuthAnyTransport struct{ user, pass string }

func (a plainAuthAnyTransport) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "PLAIN", []byte("\x00" + a.user + "\x00" + a.pass), nil
}

func (a plainAuthAnyTransport) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return nil, errors.New("unexpected server challenge")
	}
	return nil, nil
}

// ValidateAddress reports an error unless s is a single bare email
// address ("a@b.com", not "Name <a@b.com>" or a list).
func ValidateAddress(s string) error {
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s {
		return fmt.Errorf("email: %q is not a valid email address", s)
	}
	return nil
}

// FormatFrom renders "Name <addr>" (or the bare address when name is empty).
func FormatFrom(name, addr string) string {
	if name == "" {
		return addr
	}
	return (&mail.Address{Name: name, Address: addr}).String()
}

// buildSMTPMessage returns the envelope recipient and the RFC 5322 message.
func buildSMTPMessage(from, to, subject, body string) (string, []byte, error) {
	rcpt, msg, _, err := buildMessage(from, "", to, subject, body, false)
	return rcpt, msg, err
}

// buildSMTPMessageFull also returns the generated Message-ID. Every
// header value is re-serialized from a parsed address or RFC 2047-encoded,
// so no caller input can start a new header line.
func buildSMTPMessageFull(from, replyTo, to, subject, body string) (string, []byte, string, error) {
	return buildMessage(from, replyTo, to, subject, body, true)
}

func buildMessage(from, replyTo, to, subject, body string, withIDs bool) (string, []byte, string, error) {
	if err := rejectHeaderInjection(to, subject); err != nil {
		return "", nil, "", err
	}
	rcpt, err := mail.ParseAddress(to)
	if err != nil {
		return "", nil, "", fmt.Errorf("email: invalid to address: %w", err)
	}
	sender, err := mail.ParseAddress(from)
	if err != nil {
		return "", nil, "", fmt.Errorf("email: invalid from address: %w", err)
	}
	var reply *mail.Address
	if replyTo != "" {
		if reply, err = mail.ParseAddress(replyTo); err != nil {
			return "", nil, "", fmt.Errorf("email: invalid reply-to address: %w", err)
		}
	}

	domain := "localhost"
	if i := strings.LastIndex(sender.Address, "@"); i >= 0 {
		domain = sender.Address[i+1:]
	}
	raw := make([]byte, 12)
	_, _ = rand.Read(raw)
	id := "<" + hex.EncodeToString(raw) + "@" + domain + ">"

	var b strings.Builder
	if withIDs {
		b.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
		b.WriteString("Message-ID: " + id + "\r\n")
	}
	b.WriteString("To: " + (&mail.Address{Address: rcpt.Address}).String() + "\r\n")
	b.WriteString("From: " + sender.String() + "\r\n")
	if reply != nil {
		b.WriteString("Reply-To: " + reply.String() + "\r\n")
	}
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	b.WriteString("\r\n")
	return rcpt.Address, []byte(b.String()), id, nil
}
