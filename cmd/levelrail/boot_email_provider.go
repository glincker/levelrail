package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// emailConfigFromSettings maps a settings row plus its credentials onto the Config the provider registry builds a Sender from.
func emailConfigFromSettings(settings store.EmailSettings, secret func(string) (string, error)) (email.Config, error) {
	from := email.FormatFrom(settings.FromName, settings.EffectiveFrom())
	limits := email.Limits{PerMinute: settings.SendsPerMinute, PerDay: settings.SendsPerDay}
	if limits.PerMinute == 0 {
		limits.PerMinute = emailEnvInt("APP_EMAIL_SENDS_PER_MINUTE")
	}
	if limits.PerDay == 0 {
		limits.PerDay = emailEnvInt("APP_EMAIL_SENDS_PER_DAY")
	}
	smtpCfg := func(host string, port int, username, password string) *email.SMTPConfig {
		return &email.SMTPConfig{
			Addr: fmt.Sprintf("%s:%d", host, port), Host: host, Username: username, Password: password,
			From: from, EnvelopeFrom: settings.EnvelopeFrom, ReplyTo: settings.ReplyTo,
			TLSMode: settings.TLSMode, SkipVerify: settings.TLSSkipVerify,
		}
	}
	cfg := email.Config{Backend: email.Backend(settings.Backend), Limits: limits}
	switch settings.Backend {
	case store.EmailBackendSMTP:
		password, err := secret(store.EmailSecretSMTPPassword)
		if err != nil {
			return email.Config{}, err
		}
		cfg.SMTP = smtpCfg(settings.SMTPHost, settings.SMTPPort, settings.SMTPUsername, password)
	case store.EmailBackendSESSMTP:
		host, err := email.SESSMTPHost(settings.SESRegion)
		if err != nil {
			return email.Config{}, err
		}
		password, err := secret(store.EmailSecretSESSMTPPassword)
		if err != nil {
			return email.Config{}, err
		}
		port := settings.SESSMTPPort
		if port == 0 {
			port = 587
		}
		cfg.SMTP = smtpCfg(host, port, settings.SESSMTPUsername, password)
	case store.EmailBackendSES:
		key, err := secret(store.EmailSecretSESSecretKey)
		if err != nil {
			return email.Config{}, err
		}
		cfg.SES = &email.SESConfig{Region: settings.SESRegion, AccessKeyID: settings.SESAccessKeyID, SecretAccessKey: key, From: from, ReplyTo: settings.ReplyTo}
	case store.EmailBackendResend:
		key, err := secret(store.EmailSecretResendAPIKey)
		if err != nil {
			return email.Config{}, err
		}
		cfg.Resend = &email.ResendConfig{APIKey: key, From: from, ReplyTo: settings.ReplyTo}
	case store.EmailBackendPostmark, store.EmailBackendSendGrid, store.EmailBackendMailgun:
		envKey := map[string]string{
			store.EmailBackendPostmark: store.EmailSecretPostmarkToken,
			store.EmailBackendSendGrid: store.EmailSecretSendGridAPIKey,
			store.EmailBackendMailgun:  store.EmailSecretMailgunAPIKey,
		}[settings.Backend]
		key, err := secret(envKey)
		if err != nil {
			return email.Config{}, err
		}
		cfg.API = &email.APIConfig{APIKey: key, From: from, ReplyTo: settings.ReplyTo, Domain: settings.MailgunDomain, Region: settings.MailgunRegion}
	default:
		return email.Config{}, nil
	}
	return cfg, nil
}

func emailEnvInt(name string) int {
	n, err := strconv.Atoi(os.Getenv(name))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// emailDeliveryRecorder writes send outcomes to the delivery log and answers the cap counters.
type emailDeliveryRecorder struct {
	db          *store.DB
	fullAddress bool
}

func newEmailDeliveryRecorder(db *store.DB) *emailDeliveryRecorder {
	return &emailDeliveryRecorder{db: db, fullAddress: os.Getenv("APP_EMAIL_LOG_FULL_ADDRESS") == "true"}
}

func (r *emailDeliveryRecorder) RecordDelivery(ctx context.Context, d email.Delivery) error {
	errText := d.Error
	if len(errText) > 1000 {
		errText = errText[:1000]
	}
	return r.db.InsertEmailDelivery(ctx, store.EmailDelivery{
		CreatedAt: d.At, Provider: d.Provider, ProviderID: d.ProviderID, FailoverFrom: d.FailoverFrom, Kind: d.Kind, Status: d.Status, MessageID: d.MessageID,
		Error: errText, ErrorKind: string(d.ErrorKind), Attempts: d.Attempts,
	}, d.To, r.fullAddress)
}

// RecordProviderResult stamps last_ok_at / last_error_kind on the provider row.
func (r *emailDeliveryRecorder) RecordProviderResult(ctx context.Context, providerID string, ok bool, kind email.FailureKind, at time.Time) error {
	return r.db.RecordEmailProviderResult(ctx, providerID, ok, string(kind), at)
}

func (r *emailDeliveryRecorder) CountSent(ctx context.Context, since time.Time) (int, error) {
	return r.db.CountEmailDeliveriesSent(ctx, since)
}

// emailLogRetention is how long delivery log rows are kept; zero keeps them forever.
func emailLogRetention(ctx context.Context, settings *runtimesettings.Service) time.Duration {
	return time.Duration(settings.Int(ctx, runtimesettings.KeyEmailLogDays)) * 24 * time.Hour
}

// pruneEmailDeliveries is one sweep of the delivery log; a zero retention keeps everything.
func pruneEmailDeliveries(ctx context.Context, db *store.DB, retention time.Duration, now time.Time) (int64, error) {
	if retention <= 0 {
		return 0, nil
	}
	return db.PruneEmailDeliveries(ctx, now.Add(-retention))
}
