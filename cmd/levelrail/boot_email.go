package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Env-only SMTP fallback; nil is a valid result meaning not configured.
func smtpConfigFromEnv() *email.SMTPConfig {
	host := os.Getenv("APP_SMTP_HOST")
	from := os.Getenv("APP_SMTP_FROM")
	if host == "" || from == "" {
		return nil
	}
	port := os.Getenv("APP_SMTP_PORT")
	if port == "" {
		port = "587"
	}
	return &email.SMTPConfig{
		Addr:     host + ":" + port,
		Host:     host,
		Username: os.Getenv("APP_SMTP_USERNAME"),
		Password: os.Getenv("APP_SMTP_PASSWORD"),
		From:     from,
	}
}

// emailConfigLoader resolves email_settings fresh on every send,
// decrypting its stored credential through secretsManager, falling back
// to envFallback when the row's backend is unset.
func emailConfigLoader(db *store.DB, secretsManager *secrets.Manager, envFallback *email.SMTPConfig) email.ConfigLoader {
	return func(ctx context.Context) (email.Config, error) {
		settings, err := db.GetEmailSettings(ctx)
		if err != nil {
			return email.Config{}, err
		}
		if settings.Backend == "" {
			if envFallback == nil {
				return email.Config{}, nil // NewSender turns this into email.ErrNotConfigured
			}
			return email.Config{Backend: email.BackendSMTP, SMTP: envFallback}, nil
		}
		if secretsManager == nil {
			return email.Config{}, fmt.Errorf("email settings: %s backend selected but no master key is configured", settings.Backend)
		}
		return emailConfigFromSettings(settings, func(key string) (string, error) {
			return resolveEmailSecret(ctx, secretsManager, key)
		})
	}
}

// resolveEmailSecret treats "never set" as an empty string rather than
// an error: a backend can be selected before its credential is saved.
func resolveEmailSecret(ctx context.Context, secretsManager *secrets.Manager, envKey string) (string, error) {
	value, err := secretsManager.Resolve(ctx, store.EmailSettingsSecretsKey(), envKey)
	if errors.Is(err, secrets.ErrValueNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

// Persisted beside the other SQLite files; unlike the server certificate, the CA survives restarts.
const (
	agentCACertFilename = "agent-ca.crt.pem"
	agentCAKeyFilename  = "agent-ca.key.pem"

	// masterKeyFilename is the persisted secrets.MasterKey fallback used
	// when APP_MASTER_KEY isn't set, same data directory as the agent CA
	// above.
	masterKeyFilename = "master.key"

	// envMasterKeyFile points at a key file outside the data directory, so a
	// copy of the data directory alone cannot decrypt stored secrets.
	envMasterKeyFile = "APP_MASTER_KEY_FILE"
)
