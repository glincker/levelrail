package email

import (
	"context"
	"errors"
	"net/textproto"
	"testing"
	"time"
)

func TestSESSMTPHost(t *testing.T) {
	got, err := SESSMTPHost("eu-west-1")
	if err != nil || got != "email-smtp.eu-west-1.amazonaws.com" {
		t.Fatalf("SESSMTPHost(eu-west-1) = %q, %v", got, err)
	}
	if _, err := SESSMTPHost("email-smtp.us-east-1.amazonaws.com"); err == nil {
		t.Fatal("a hostname must not pass as a region")
	}
}

func TestValidSESRegion(t *testing.T) {
	for region, want := range map[string]bool{"us-east-1": true, "ap-southeast-2": true, "": false, "mars-north-1": false, "US-EAST-1": false} {
		if got := ValidSESRegion(region); got != want {
			t.Errorf("ValidSESRegion(%q) = %v, want %v", region, got, want)
		}
	}
}

func TestProviderRegistry(t *testing.T) {
	cases := map[Backend]Config{
		BackendSMTP:     {SMTP: &SMTPConfig{Addr: "h:587", Host: "h", From: "a@b.co"}},
		BackendSESSMTP:  {SMTP: &SMTPConfig{Addr: "email-smtp.us-east-1.amazonaws.com:587", Host: "email-smtp.us-east-1.amazonaws.com", From: "a@b.co"}},
		BackendSES:      {SES: &SESConfig{Region: "us-east-1", From: "a@b.co"}},
		BackendResend:   {Resend: &ResendConfig{APIKey: "k", From: "a@b.co"}},
		BackendPostmark: {API: &APIConfig{APIKey: "k", From: "a@b.co"}},
		BackendSendGrid: {API: &APIConfig{APIKey: "k", From: "a@b.co"}},
		BackendMailgun:  {API: &APIConfig{APIKey: "k", From: "a@b.co", Domain: "mg.b.co"}},
	}
	for b, cfg := range cases {
		cfg.Backend = b
		if s, err := NewSender(cfg); err != nil || s == nil {
			t.Errorf("NewSender(%s) = %v, %v", b, s, err)
		}
		if !ValidBackend(b) {
			t.Errorf("ValidBackend(%s) = false", b)
		}
	}
	if _, err := NewSender(Config{Backend: BackendMailgun, API: &APIConfig{APIKey: "k"}}); err == nil {
		t.Error("mailgun without a domain must not build")
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		err  error
		want FailureKind
	}{
		{&textproto.Error{Code: 535, Msg: "Authentication Credentials Invalid"}, FailAuth},
		{&HTTPError{Provider: "postmark", Status: 422, Body: "Sender signature not found"}, FailSenderNotVerified},
		{errors.New("MessageRejected: Email address is not verified. The following identities failed the check in region EU-WEST-1: a@b.co"), FailRegionMismatch},
		{&HTTPError{Provider: "resend", Status: 429, Body: "slow down"}, FailRateLimited},
		{errors.New("starttls: tls: handshake failure"), FailTLS},
		{ErrRateLimited, FailRateLimited},
	}
	for _, c := range cases {
		if got := Classify(c.err); got != c.want {
			t.Errorf("Classify(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}

type flakySender struct{ failures, calls int }

func (f *flakySender) Send(context.Context, string, string, string) error {
	f.calls++
	if f.calls <= f.failures {
		return &HTTPError{Provider: "x", Status: 503, Body: "busy"}
	}
	return nil
}

type memRecorder struct{ rows []Delivery }

func (m *memRecorder) RecordDelivery(_ context.Context, d Delivery) error {
	m.rows = append(m.rows, d)
	return nil
}
func (m *memRecorder) CountSent(context.Context, time.Time) (int, error) { return len(m.rows), nil }

func TestDynamicSender_RetriesTransientAndLogs(t *testing.T) {
	flaky := &flakySender{failures: 2}
	registry["flaky"] = func(Config) (Sender, error) { return flaky, nil }
	t.Cleanup(func() { delete(registry, "flaky") })

	rec := &memRecorder{}
	d := NewDynamicSender(func(context.Context) (Config, error) { return Config{Backend: "flaky"}, nil }).WithRecorder(rec).WithRetry(3, time.Millisecond)
	res, err := d.SendDetailed(context.Background(), "to@example.com", "s", "b")
	if err != nil || res.Attempts != 3 || flaky.calls != 3 {
		t.Fatalf("SendDetailed() = %+v, %v after %d calls, want success on the 3rd attempt", res, err, flaky.calls)
	}
	if len(rec.rows) != 1 || rec.rows[0].Status != DeliverySent {
		t.Fatalf("recorded = %+v, want one sent row", rec.rows)
	}
}

func TestDynamicSender_EnforcesPerMinuteCap(t *testing.T) {
	registry["flaky"] = func(Config) (Sender, error) { return &flakySender{}, nil }
	t.Cleanup(func() { delete(registry, "flaky") })

	rec := &memRecorder{rows: []Delivery{{Status: DeliverySent}, {Status: DeliverySent}}}
	d := NewDynamicSender(func(context.Context) (Config, error) {
		return Config{Backend: "flaky", Limits: Limits{PerMinute: 2}}, nil
	}).WithRecorder(rec)
	if err := d.Send(context.Background(), "to@example.com", "s", "b"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("Send() error = %v, want ErrRateLimited", err)
	}
}

func TestDynamicSender_SendMessageIsLogged(t *testing.T) {
	registry["flaky"] = func(Config) (Sender, error) { return &flakySender{}, nil }
	t.Cleanup(func() { delete(registry, "flaky") })

	rec := &memRecorder{}
	d := NewDynamicSender(func(context.Context) (Config, error) { return Config{Backend: "flaky"}, nil }).WithRecorder(rec)
	if err := d.SendMessage(context.Background(), Message{To: "to@example.com", Subject: "s", Text: "b", HTML: "<p>b</p>"}); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if len(rec.rows) != 1 || rec.rows[0].Status != DeliverySent {
		t.Fatalf("recorded = %+v, want one sent row for a rich send", rec.rows)
	}
}
