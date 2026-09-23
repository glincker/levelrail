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

func TestBuildSMTPMessage_NoInjectedHeaders(t *testing.T) {
	tests := []struct {
		name, to, subject string
	}{
		{"crlf in to", "victim@example.com\r\nBcc: attacker@evil.com", "hi"},
		{"lf in subject", "victim@example.com", "hi\nBcc: attacker@evil.com"},
		{"two recipients", "victim@example.com, attacker@evil.com", "hi"},
		{"not an address", "not an address", "hi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := buildSMTPMessage("a@example.com", tt.to, tt.subject, "body"); err == nil {
				t.Fatalf("buildSMTPMessage(%q, %q) error = nil, want a rejection", tt.to, tt.subject)
			}
		})
	}
}

func TestBuildSMTPMessage_EncodesSubjectAndNormalizesTo(t *testing.T) {
	rcpt, msg, err := buildSMTPMessage("Ops <ops@example.com>", "Victim <victim@example.com>", "Deploy \u00e9chou\u00e9", "line one\nBcc: not-a-header@evil.com")
	if err != nil {
		t.Fatalf("buildSMTPMessage() error = %v", err)
	}
	if rcpt != "victim@example.com" {
		t.Errorf("rcpt = %q, want the bare parsed address", rcpt)
	}
	head, _, ok := strings.Cut(string(msg), "\r\n\r\n")
	if !ok {
		t.Fatalf("message has no header/body separator: %q", msg)
	}
	want := []string{
		"To: <victim@example.com>",
		`From: "Ops" <ops@example.com>`,
		"Subject: =?utf-8?q?Deploy_=C3=A9chou=C3=A9?=",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
	}
	if got := strings.Split(head, "\r\n"); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("headers = %q, want %q", got, want)
	}
}

func TestValidateAddress(t *testing.T) {
	for _, ok := range []string{"a@example.com", "first.last+tag@sub.example.org"} {
		if err := ValidateAddress(ok); err != nil {
			t.Errorf("ValidateAddress(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "admin", "Name <a@example.com>", "a@example.com, b@example.com", "a@example.com\r\nBcc: b@example.com"} {
		if err := ValidateAddress(bad); err == nil {
			t.Errorf("ValidateAddress(%q) = nil, want an error", bad)
		}
	}
}
