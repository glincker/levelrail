package email

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewSender_Unconfigured_ReturnsErrNotConfigured(t *testing.T) {
	_, err := NewSender(Config{})
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("error = %v, want ErrNotConfigured", err)
	}
}

func TestNewSender_UnknownBackend_ReturnsErrNotConfigured(t *testing.T) {
	_, err := NewSender(Config{Backend: "smtp2"})
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("error = %v, want ErrNotConfigured", err)
	}
}

func TestNewSender_SMTP_NoConfig_Errors(t *testing.T) {
	_, err := NewSender(Config{Backend: BackendSMTP})
	if err == nil {
		t.Fatal("error = nil, want an error when Backend is smtp but SMTP is nil")
	}
}

func TestNewSender_SES_NoConfig_Errors(t *testing.T) {
	_, err := NewSender(Config{Backend: BackendSES})
	if err == nil {
		t.Fatal("error = nil, want an error when Backend is ses but SES is nil")
	}
}

func TestNewSender_SMTP_ReturnsSender(t *testing.T) {
	s, err := NewSender(Config{Backend: BackendSMTP, SMTP: &SMTPConfig{Addr: "127.0.0.1:1", Host: "127.0.0.1", From: "a@example.com"}})
	if err != nil {
		t.Fatalf("NewSender() error = %v", err)
	}
	if s == nil {
		t.Fatal("NewSender() sender = nil")
	}
}

func TestNewSender_SES_ReturnsSender(t *testing.T) {
	s, err := NewSender(Config{Backend: BackendSES, SES: &SESConfig{Region: "us-east-1", AccessKeyID: "AKID", SecretAccessKey: "secret", From: "a@example.com"}})
	if err != nil {
		t.Fatalf("NewSender() error = %v", err)
	}
	if s == nil {
		t.Fatal("NewSender() sender = nil")
	}
}

func TestSMTPSender_UnreachableServer_ErrorPropagates(t *testing.T) {
	s, err := NewSender(Config{Backend: BackendSMTP, SMTP: &SMTPConfig{Addr: "127.0.0.1:1", Host: "127.0.0.1", From: "a@example.com"}})
	if err != nil {
		t.Fatalf("NewSender() error = %v", err)
	}
	err = s.Send(context.Background(), "to@example.com", "subject", "body")
	if err == nil {
		t.Fatal("Send() error = nil, want an error when the SMTP server is unreachable")
	}
	if !strings.Contains(err.Error(), "send email") {
		t.Errorf("error = %q, want it wrapped with %q", err.Error(), "send email")
	}
}

func TestDynamicSender_LoaderError_Propagates(t *testing.T) {
	wantErr := errors.New("boom")
	d := NewDynamicSender(func(context.Context) (Config, error) { return Config{}, wantErr })
	err := d.Send(context.Background(), "to@example.com", "s", "b")
	if err == nil || !errors.Is(err, wantErr) {
		t.Errorf("Send() error = %v, want it to wrap %v", err, wantErr)
	}
}

func TestDynamicSender_NoBackendConfigured_ReturnsErrNotConfigured(t *testing.T) {
	d := NewDynamicSender(func(context.Context) (Config, error) { return Config{}, nil })
	err := d.Send(context.Background(), "to@example.com", "s", "b")
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Send() error = %v, want ErrNotConfigured", err)
	}
}

func TestDynamicSender_ReResolvesConfigEveryCall(t *testing.T) {
	calls := 0
	d := NewDynamicSender(func(context.Context) (Config, error) {
		calls++
		return Config{Backend: BackendSMTP, SMTP: &SMTPConfig{Addr: "127.0.0.1:1", Host: "127.0.0.1", From: "a@example.com"}}, nil
	})
	_ = d.Send(context.Background(), "to@example.com", "s", "b")
	_ = d.Send(context.Background(), "to@example.com", "s", "b")
	if calls != 2 {
		t.Errorf("loader calls = %d, want 2 (Send must re-resolve Config every call, never cache it)", calls)
	}
}
