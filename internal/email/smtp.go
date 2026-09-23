package email

import (
	"context"
	"fmt"
	"mime"
	"net/mail"
	"net/smtp"
	"strings"
)

// smtpSender sends via net/smtp. net/smtp.SendMail takes no
// context.Context, so a hung connection isn't cancellable via ctx.
type smtpSender struct {
	cfg SMTPConfig
}

func (s smtpSender) Send(_ context.Context, to, subject, body string) error {
	rcpt, msg, err := buildSMTPMessage(s.cfg.From, to, subject, body)
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

// ValidateAddress reports an error unless s is a single bare email
// address ("a@b.com", not "Name <a@b.com>" or a list).
func ValidateAddress(s string) error {
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s {
		return fmt.Errorf("email: %q is not a valid email address", s)
	}
	return nil
}

// buildSMTPMessage returns the envelope recipient and the RFC 5322
// message. Every header value is re-serialized from a parsed address or
// RFC 2047-encoded, so no caller input can start a new header line.
func buildSMTPMessage(from, to, subject, body string) (string, []byte, error) {
	if err := rejectHeaderInjection(to, subject); err != nil {
		return "", nil, err
	}
	rcpt, err := mail.ParseAddress(to)
	if err != nil {
		return "", nil, fmt.Errorf("email: invalid to address: %w", err)
	}
	sender, err := mail.ParseAddress(from)
	if err != nil {
		return "", nil, fmt.Errorf("email: invalid from address: %w", err)
	}

	var b strings.Builder
	b.WriteString("To: " + (&mail.Address{Address: rcpt.Address}).String() + "\r\n")
	b.WriteString("From: " + sender.String() + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	b.WriteString("\r\n")
	return rcpt.Address, []byte(b.String()), nil
}
