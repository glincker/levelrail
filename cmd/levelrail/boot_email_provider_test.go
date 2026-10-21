package main

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestEmailConfigFromSettings_SESSMTPDerivesHostFromRegion(t *testing.T) {
	settings := store.EmailSettings{
		Backend: store.EmailBackendSESSMTP, SESRegion: "eu-central-1", SESSMTPUsername: "AKSMTP",
		SESSMTPPort: 465, FromAddress: "alerts@example.com", FromName: "Ops",
	}
	cfg, err := emailConfigFromSettings(settings, func(string) (string, error) { return "smtp-secret", nil })
	if err != nil {
		t.Fatalf("emailConfigFromSettings() error = %v", err)
	}
	if cfg.Backend != email.BackendSESSMTP || cfg.SMTP == nil || cfg.SMTP.Addr != "email-smtp.eu-central-1.amazonaws.com:465" {
		t.Fatalf("config = %+v, want ses_smtp with a derived SMTP address", cfg)
	}
	if cfg.SMTP.From != `"Ops" <alerts@example.com>` {
		t.Errorf("from = %q, want the display name applied", cfg.SMTP.From)
	}
}
