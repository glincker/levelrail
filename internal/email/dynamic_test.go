package email

import (
	"context"
	"strings"
	"testing"
)

func TestRejectHeaderInjection_CatchesCRLFInTo(t *testing.T) {
	cases := []string{
		"victim@example.com\r\nBcc: attacker@evil.com",
		"victim@example.com\nBcc: attacker@evil.com",
		"victim@example.com\rBcc: attacker@evil.com",
	}
	for _, to := range cases {
		if err := rejectHeaderInjection(to, "subject"); err == nil {
			t.Errorf("rejectHeaderInjection(to=%q) error = nil, want a rejection", to)
		}
	}
}

func TestRejectHeaderInjection_CatchesCRLFInSubject(t *testing.T) {
	if err := rejectHeaderInjection("user@example.com", "Hi\r\nBcc: attacker@evil.com"); err == nil {
		t.Error("rejectHeaderInjection() error = nil, want a rejection for a newline in subject")
	}
}

func TestRejectHeaderInjection_AllowsOrdinaryInput(t *testing.T) {
	if err := rejectHeaderInjection("user@example.com", "A normal subject"); err != nil {
		t.Errorf("rejectHeaderInjection() error = %v, want nil for ordinary input", err)
	}
}

func TestDynamicSender_Send_RejectsBeforeLoadingConfig(t *testing.T) {
	loadCalled := false
	d := NewDynamicSender(func(context.Context) (Config, error) {
		loadCalled = true
		return Config{}, nil
	})

	err := d.Send(context.Background(), "victim@example.com\r\nBcc: attacker@evil.com", "subject", "body")
	if err == nil {
		t.Fatal("Send() error = nil, want a rejection")
	}
	if loadCalled {
		t.Error("ConfigLoader was called after an invalid to/subject; the rejection should short-circuit before any backend work")
	}
}

func TestSMTPSender_RawMessage_HeaderInjectionWouldOtherwiseSucceed(t *testing.T) {
	// Documents why the guard lives in DynamicSender.Send rather than
	// smtpSender: net/smtp.SendMail has no header-aware validation of
	// its own, so smtpSender alone would happily hand a CRLF-carrying
	// "to" straight into the raw message it builds.
	to := "victim@example.com\r\nBcc: attacker@evil.com"
	msg := "To: " + to + "\r\nFrom: a@example.com\r\nSubject: hi\r\n\r\nbody\r\n"
	if !strings.Contains(msg, "Bcc: attacker@evil.com") {
		t.Fatal("expected the naive message construction to demonstrate the injected header")
	}
}
