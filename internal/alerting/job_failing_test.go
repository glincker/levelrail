package alerting

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestJobFailureNotice_NameAndExitCodeOnly(t *testing.T) {
	code := 137
	got := JobFailureNotice(store.JobFailureAlert{TaskID: "st_1", ServiceName: "web\r\nX-Injected: 1", Name: "nightly\nfake line", Status: "failed", ExitCode: &code})
	if !strings.Contains(got, "nightlyfake line") || !strings.Contains(got, "exit code 137") {
		t.Fatalf("notice = %q, want the cleaned name and exit code", got)
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("notice carries control characters: %q", got)
	}
	if none := JobFailureNotice(store.JobFailureAlert{TaskID: "st_2", ServiceName: "web", Status: "timeout"}); !strings.Contains(none, `"st_2"`) || !strings.Contains(none, "exit code none") {
		t.Fatalf("notice without name or code = %q", none)
	}
}

type fakeJobFailing struct {
	due      map[string][]store.JobFailureAlert
	services []string
	released int
}

func (f *fakeJobFailing) ClaimJobFailureAlerts(_ context.Context, ruleID, service string, _, _ time.Time, _ int) ([]store.JobFailureAlert, error) {
	f.services = append(f.services, service)
	out := f.due[ruleID]
	f.due[ruleID] = nil
	return out, nil
}

func (f *fakeJobFailing) ReleaseJobFailureAlert(context.Context, string, string, time.Time) error {
	f.released++
	return nil
}

type recordingNotifier struct {
	sent *[]Event
	err  error
}

func (n recordingNotifier) Notify(_ context.Context, ev Event) error {
	*n.sent = append(*n.sent, ev)
	return n.err
}

func TestTickJobFailing_ScopedRuleFiresAndUnscopedDoesNot(t *testing.T) {
	var sent []Event
	src := &fakeJobFailing{due: map[string][]store.JobFailureAlert{
		"r_scoped": {{TaskID: "st_1", ServiceName: "web", Name: "nightly", Status: "failed"}},
		"r_odd":    {{TaskID: "st_1", ServiceName: "web", Name: "nightly", Status: "failed"}},
	}}
	e := &Engine{logger: slog.Default(), newNotifier: func(Rule) Notifier { return recordingNotifier{sent: &sent} }}
	e.SetJobFailing(src, func() time.Duration { return time.Hour }, func() int { return 10 })
	now := time.Now()
	if err := e.tickJobFailing(context.Background(), Rule{ID: "r_scoped", Kind: KindJobFailing, ResourceID: "service:web", NotifyURL: "https://example.invalid/hook"}, now); err != nil {
		t.Fatal(err)
	}
	if err := e.tickJobFailing(context.Background(), Rule{ID: "r_odd", Kind: KindJobFailing, ResourceID: "web", NotifyURL: "https://example.invalid/hook"}, now); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || !strings.Contains(sent[0].TaskFailureNotice, "nightly") {
		t.Fatalf("sent = %+v, want one notice from the scoped rule", sent)
	}
	if len(src.services) != 1 || src.services[0] != "web" {
		t.Fatalf("claimed services = %v, want only [web]", src.services)
	}
}

func TestTickJobFailing_FailedSendReleasesClaim(t *testing.T) {
	var sent []Event
	src := &fakeJobFailing{due: map[string][]store.JobFailureAlert{"r1": {{TaskID: "st_1", ServiceName: "web", Status: "failed"}}}}
	e := &Engine{logger: slog.Default(), newNotifier: func(Rule) Notifier { return recordingNotifier{sent: &sent, err: errors.New("channel down")} }}
	e.SetJobFailing(src, func() time.Duration { return time.Hour }, func() int { return 10 })
	_ = e.tickJobFailing(context.Background(), Rule{ID: "r1", Kind: KindJobFailing, ResourceID: "service:web", NotifyURL: "https://example.invalid/hook"}, time.Now())
	if src.released != 1 {
		t.Fatalf("released = %d, want 1 after a failed send", src.released)
	}
}

func TestCleanAlertText_CutsOnRuneBoundary(t *testing.T) {
	got := cleanAlertText(strings.Repeat("a", 127) + "éé")
	if !utf8.ValidString(got) || len(got) > 128 {
		t.Fatalf("cleanAlertText = %q (len %d), want valid UTF-8 within 128 bytes", got, len(got))
	}
}
