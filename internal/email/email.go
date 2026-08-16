// Package email is the platform's one email-sending capability: a
// narrow Sender interface plus SMTP and SES implementations, shared by
// internal/alerting and internal/api so neither imports the other.
package email

import (
	"context"
	"errors"
	"fmt"
)

// Backend names which transport a Config uses to actually send.
type Backend string

// The two backends NewSender supports.
const (
	BackendSMTP Backend = "smtp"
	BackendSES  Backend = "ses"
)

// ErrNotConfigured is returned by NewSender when a Config names no
// backend: no settings row and no env-var fallback.
var ErrNotConfigured = errors.New("email: not configured")

// SMTPConfig is an SMTP server's connection details.
type SMTPConfig struct {
	// Addr is host:port, e.g. "smtp.example.com:587".
	Addr     string
	Host     string // just the host part of Addr, for PlainAuth's identity check
	Username string
	Password string
	From     string
}

// SESConfig is an AWS SES v2 sender's connection details.
type SESConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	From            string
}

// Config names which backend to use plus that backend's own settings.
type Config struct {
	Backend Backend
	SMTP    *SMTPConfig
	SES     *SESConfig
}

// Sender sends one plain-text email.
type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// NewSender builds the Sender cfg.Backend names, or ErrNotConfigured for
// an empty or unrecognized Backend.
func NewSender(cfg Config) (Sender, error) {
	switch cfg.Backend {
	case BackendSMTP:
		if cfg.SMTP == nil {
			return nil, fmt.Errorf("email: smtp backend selected with no SMTP config")
		}
		return smtpSender{cfg: *cfg.SMTP}, nil
	case BackendSES:
		if cfg.SES == nil {
			return nil, fmt.Errorf("email: ses backend selected with no SES config")
		}
		return sesSender{cfg: *cfg.SES}, nil
	default:
		return nil, ErrNotConfigured
	}
}
