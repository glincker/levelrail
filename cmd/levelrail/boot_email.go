package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

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

// emailProvidersLoader returns the ordered provider list: the primary, then the enabled backup. With no provider
// rows it defers to the single-row and env path, so one provider or none behaves exactly as before.
func emailProvidersLoader(db *store.DB, secretsManager *secrets.Manager, envFallback *email.SMTPConfig) email.ProvidersLoader {
	single := emailConfigLoader(db, secretsManager, envFallback)
	return func(ctx context.Context) ([]email.ProviderConfig, error) {
		if only := email.OnlyProviderID(ctx); only != "" {
			return onlyProviderConfig(ctx, db, secretsManager, only)
		}
		rows, err := db.ListEmailProviders(ctx)
		if err != nil {
			return nil, err
		}
		var enabled []store.EmailProvider
		for _, r := range rows {
			if r.Enabled {
				enabled = append(enabled, r)
			}
		}
		if len(enabled) == 0 {
			cfg, err := single(ctx)
			if err != nil {
				return nil, err
			}
			return []email.ProviderConfig{{Config: cfg}}, nil
		}
		if secretsManager == nil {
			return nil, fmt.Errorf("email providers: no master key is configured")
		}
		settings, err := db.GetEmailSettings(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]email.ProviderConfig, 0, 2)
		for _, r := range enabled {
			cfg, err := emailConfigFromSettings(providerSettings(settings, r), func(key string) (string, error) {
				return resolveEmailProviderSecret(ctx, secretsManager, r, key)
			})
			if err != nil {
				if len(out) == 0 {
					return nil, err
				}
				continue // a broken backup must not take the primary down with it
			}
			out = append(out, email.ProviderConfig{ID: r.ID, Config: cfg})
			if len(out) == 2 {
				break
			}
		}
		return out, nil
	}
}

// onlyProviderConfig resolves one provider by id, enabled or not, for a test send that must not fail over.
func onlyProviderConfig(ctx context.Context, db *store.DB, m *secrets.Manager, id string) ([]email.ProviderConfig, error) {
	if m == nil {
		return nil, fmt.Errorf("email providers: no master key is configured")
	}
	r, err := db.GetEmailProvider(ctx, id)
	if err != nil {
		return nil, err
	}
	settings, err := db.GetEmailSettings(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := emailConfigFromSettings(providerSettings(settings, r), func(key string) (string, error) {
		return resolveEmailProviderSecret(ctx, m, r, key)
	})
	if err != nil {
		return nil, err
	}
	return []email.ProviderConfig{{ID: r.ID, Config: cfg}}, nil
}

// providerSettings overlays a provider row's transport fields on the shared sender identity and caps.
func providerSettings(shared store.EmailSettings, p store.EmailProvider) store.EmailSettings {
	s := shared
	s.Backend, s.SMTPHost, s.SMTPPort, s.SMTPUsername, s.SMTPFrom = p.Backend, p.SMTPHost, p.SMTPPort, p.SMTPUsername, p.SMTPFrom
	s.SESRegion, s.SESAccessKeyID, s.SESFrom, s.ResendFrom = p.SESRegion, p.SESAccessKeyID, p.SESFrom, p.ResendFrom
	s.TLSMode, s.TLSSkipVerify = p.TLSMode, p.TLSSkipVerify
	s.SESSMTPPort, s.SESSMTPUsername, s.MailgunDomain, s.MailgunRegion = p.SESSMTPPort, p.SESSMTPUsername, p.MailgunDomain, p.MailgunRegion
	return s
}

// resolveEmailProviderSecret reads the primary's credentials from the shared email-settings service name
// (unchanged) and every other provider's from its own.
func resolveEmailProviderSecret(ctx context.Context, m *secrets.Manager, p store.EmailProvider, envKey string) (string, error) {
	value, err := m.Resolve(ctx, store.EmailProviderSecretsKey(p.Role, p.ID), envKey)
	if errors.Is(err, secrets.ErrValueNotFound) {
		return "", nil
	}
	return value, err
}

// emailFailoverFromEnv reads the circuit breaker settings; zero values select the package defaults.
func emailFailoverFromEnv() (int, time.Duration) {
	return emailEnvInt("APP_EMAIL_FAILOVER_THRESHOLD"), time.Duration(emailEnvInt("APP_EMAIL_FAILOVER_COOLDOWN_SECONDS")) * time.Second
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
