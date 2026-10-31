// Package email is the platform's one email-sending capability: a
// narrow Sender interface plus a registry of providers (SMTP, SES,
// Resend, Postmark, SendGrid, Mailgun), shared by internal/alerting and
// internal/api so neither imports the other.
package email

import (
	"context"
	"errors"
	"fmt"
)

// Backend names which transport a Config uses to actually send.
type Backend string

// The built-in backends; each registers itself in its own file.
const (
	BackendSMTP     Backend = "smtp"
	BackendSES      Backend = "ses"
	BackendSESSMTP  Backend = "ses_smtp"
	BackendResend   Backend = "resend"
	BackendPostmark Backend = "postmark"
	BackendSendGrid Backend = "sendgrid"
	BackendMailgun  Backend = "mailgun"
)

// ErrNotConfigured is returned by NewSender when a Config names no
// backend: no settings row and no env-var fallback.
var ErrNotConfigured = errors.New("email: not configured")

// TLS modes for SMTP-based backends.
const (
	TLSAuto     = ""         // STARTTLS when the server offers it, implicit TLS on port 465
	TLSStartTLS = "starttls" // require STARTTLS
	TLSImplicit = "tls"      // TLS from the first byte
	TLSNone     = "none"     // plaintext
)

// SMTPConfig is an SMTP server's connection details.
type SMTPConfig struct {
	// Addr is host:port, e.g. "smtp.example.com:587".
	Addr     string
	Host     string // just the host part of Addr, for PlainAuth's identity check
	Username string
	Password string
	From     string
	// EnvelopeFrom is the bare MAIL FROM address; empty uses From.
	EnvelopeFrom string
	ReplyTo      string
	TLSMode      string
	SkipVerify   bool
	// PinSHA256 trusts exactly one leaf certificate (hex SHA-256 of its DER) instead of the CA chain.
	PinSHA256 string
}

// SESConfig is an AWS SES v2 sender's connection details.
type SESConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	From            string
	ReplyTo         string
}

// APIConfig is the connection detail shared by the key-based HTTP providers (Postmark, SendGrid, Mailgun).
type APIConfig struct {
	APIKey  string
	From    string
	ReplyTo string
	// Domain and Region are Mailgun-only.
	Domain string
	Region string
}

// Limits caps how many sends the control plane will make; zero means unlimited.
type Limits struct {
	PerMinute int
	PerDay    int
}

// Config names which backend to use plus that backend's own settings.
type Config struct {
	Backend Backend
	SMTP    *SMTPConfig
	SES     *SESConfig
	Resend  *ResendConfig
	API     *APIConfig
	Limits  Limits
}

// Sender sends one plain-text email.
type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// idSender is implemented by providers that can report the provider's message id.
type idSender interface {
	sendID(ctx context.Context, to, subject, body string) (string, error)
}

// SendWithID sends through s and returns the provider message id when the sender exposes one.
func SendWithID(ctx context.Context, s Sender, to, subject, body string) (string, error) {
	if is, ok := s.(idSender); ok {
		return is.sendID(ctx, to, subject, body)
	}
	return "", s.Send(ctx, to, subject, body)
}

// NewSender builds the Sender cfg.Backend names, or ErrNotConfigured for
// an empty or unrecognized Backend.
func NewSender(cfg Config) (Sender, error) {
	build, ok := registry[cfg.Backend]
	if !ok {
		return nil, ErrNotConfigured
	}
	return build(cfg)
}

func init() {
	Register(BackendSMTP, func(cfg Config) (Sender, error) {
		if cfg.SMTP == nil {
			return nil, fmt.Errorf("email: smtp backend selected with no SMTP config")
		}
		return smtpSender{cfg: *cfg.SMTP}, nil
	})
	Register(BackendSESSMTP, func(cfg Config) (Sender, error) {
		if cfg.SMTP == nil {
			return nil, fmt.Errorf("email: ses_smtp backend selected with no SMTP config")
		}
		return smtpSender{cfg: *cfg.SMTP}, nil
	})
	Register(BackendSES, func(cfg Config) (Sender, error) {
		if cfg.SES == nil {
			return nil, fmt.Errorf("email: ses backend selected with no SES config")
		}
		return sesSender{cfg: *cfg.SES}, nil
	})
	Register(BackendResend, func(cfg Config) (Sender, error) {
		if cfg.Resend == nil {
			return nil, fmt.Errorf("email: resend backend selected with no Resend config")
		}
		return resendSender{cfg: *cfg.Resend}, nil
	})
}
