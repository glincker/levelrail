// Package email is the platform's one general email-sending capability:
// a small, narrow Sender interface plus SMTP and AWS SES
// implementations, shared by internal/alerting (alert-rule and
// deploy-outcome notifications) and internal/api (password-reset
// emails) without either package importing the other's internals. Both
// depend on this package instead.
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

// ErrNotConfigured is returned by NewSender (and, through it, by
// DynamicSender.Send) when a Config names no backend at all: neither a
// settings row nor an env-var fallback names one. Every real caller
// (alerting.emailNotifier, internal/api's forgot-password handler) wraps
// this in its own caller-specific "email is not configured" message, the
// same division of labor alerting.emailNotifier.Notify already
// established before this package existed.
var ErrNotConfigured = errors.New("email: not configured")

// SMTPConfig is an SMTP server's connection details: the same shape
// alerting.SMTPConfig held before this package existed.
type SMTPConfig struct {
	// Addr is host:port, e.g. "smtp.example.com:587".
	Addr     string
	Host     string // just the host part of Addr, for PlainAuth's identity check
	Username string
	Password string
	From     string
}

// SESConfig is an AWS SES v2 sender's connection details: static
// credentials scoped to this one purpose, never the ambient environment
// or an instance role, the same reasoning internal/backup's
// newS3Client already establishes for a backup target's own credentials.
type SESConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	From            string
}

// Config names which backend to use plus that backend's own settings.
// Only the field Backend names needs to be populated.
type Config struct {
	Backend Backend
	SMTP    *SMTPConfig
	SES     *SESConfig
}

// Sender sends one plain-text email. internal/alerting and internal/api
// both depend on this interface, never on each other's internals or on
// which concrete backend is behind it.
type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// NewSender builds the Sender cfg.Backend names. Returns ErrNotConfigured
// for an empty or unrecognized Backend, so a caller with no settings row
// and no env fallback gets one clear, consistent error rather than a nil
// Sender or a panic.
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
