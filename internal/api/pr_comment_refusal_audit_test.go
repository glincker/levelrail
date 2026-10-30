package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// advancingClock moves the fixture's clock only when step is called.
func advancingClock(f *prCmdFixture) func(time.Duration) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	f.rt.prCommands.now = func() time.Time { return now }
	return func(d time.Duration) { now = now.Add(d) }
}

func TestPRCommentRefusalAudit_FloodIsWindowed(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), false)
	step := advancingClock(f)
	const n = 25
	for range n {
		f.send(commentOpts{body: "/levelrail status"})
	}
	if got := f.refusals(); len(got) != 1 || got[0].Reason != prRefusedCommandsOff || got[0].Suppressed != 0 {
		t.Fatalf("flood rows = %+v, want one commands_off row", got)
	}
	step(defaultPRCommentRefusalAuditWindow)
	f.send(commentOpts{body: "/levelrail status"})
	got := f.refusals()
	if len(got) != 2 {
		t.Fatalf("rows after the window = %d, want 2", len(got))
	}
	var counted int
	for _, d := range got {
		counted += d.Suppressed
	}
	if counted != n-1 {
		t.Errorf("suppressed count = %d, want %d", counted, n-1)
	}
}

func TestPRCommentRefusalAudit_WindowFromEnv(t *testing.T) {
	t.Setenv(envPRCommentRefusalAuditWindow, "1s")
	f := newPRCmdFixture(t, testPRCommentConfig(), false)
	step := advancingClock(f)
	for range 3 {
		f.send(commentOpts{body: "/levelrail status"})
		step(time.Second)
	}
	if got := f.refusals(); len(got) != 3 {
		t.Fatalf("rows = %d, want one per 1s window", len(got))
	}
}

func TestPRCommentRefusalAudit_DuplicateDeliveryNoExtraRow(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), false)
	step := advancingClock(f)
	f.send(commentOpts{id: 42, body: "/levelrail status"})
	step(defaultPRCommentRefusalAuditWindow)
	f.send(commentOpts{id: 42, body: "/levelrail status"})
	if got := f.refusals(); len(got) != 1 {
		t.Fatalf("rows = %+v, want one for a redelivered comment", got)
	}
}

func TestPRCommentRefusalAudit_UnconfiguredWritesNothing(t *testing.T) {
	f := newPRCmdFixture(t, PRCommentCommandConfig{}, false)
	if f.rt.prCommands != nil {
		t.Fatal("an empty config must leave the feature unconfigured")
	}
	for range 5 {
		f.send(commentOpts{body: "/levelrail status"})
	}
	if got := f.refusals(); len(got) != 0 {
		t.Fatalf("rows = %+v, want none when the feature is not configured", got)
	}
}

func TestPRCommentRefusalAudit_PermissionFailureOncePerWindow(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), true)
	f.gh.perms["octo"] = "read"
	step := advancingClock(f)
	for range 3 {
		f.send(commentOpts{body: "/levelrail redeploy"})
	}
	if got := f.refusals(); len(got) != 1 || got[0].Reason != prRefusedNoWriteAccess {
		t.Fatalf("rows = %+v, want one no_write_access row", got)
	}
	step(defaultPRCommentRefusalAuditWindow)
	f.send(commentOpts{body: "/levelrail redeploy"})
	got := f.refusals()
	if len(got) != 2 {
		t.Fatalf("rows after the window = %d, want 2", len(got))
	}
	if got[0].Suppressed+got[1].Suppressed != 2 {
		t.Errorf("suppressed = %+v, want 2 carried", got)
	}
	if len(f.builder.calls) != 0 {
		t.Fatal("a refused command deployed")
	}
}

// A harmless refusal must not hide a later write-access refusal in its window.
func TestPRCommentRefusalAudit_KeyedByReason(t *testing.T) {
	t.Setenv(envPRCommentMaxBodyBytes, "64")
	f := newPRCmdFixture(t, testPRCommentConfig(), true)
	f.gh.perms["octo"] = "read"
	advancingClock(f)
	f.send(commentOpts{body: "/levelrail status " + strings.Repeat("x", 80)})
	f.send(commentOpts{body: "/levelrail redeploy"})
	got := f.refusals()
	reasons := map[string]bool{}
	for _, d := range got {
		reasons[d.Reason] = true
	}
	if len(got) != 2 || !reasons[prRefusedBodyTooLarge] || !reasons[prRefusedNoWriteAccess] {
		t.Fatalf("rows = %+v, want one body_too_large and one no_write_access", got)
	}
}

func assertSummaryRow(t *testing.T, got []prCommandRefusalDetail, login string, want int) {
	t.Helper()
	for _, d := range got {
		if d.Login == login && d.Note == prRefusalWindowSummaryNote {
			if d.Suppressed != want {
				t.Fatalf("summary suppressed = %d, want %d", d.Suppressed, want)
			}
			return
		}
	}
	t.Fatalf("rows = %+v, want a summary row for %s", got, login)
}

// A login that goes quiet still gets its suppressed count written.
func TestPRCommentRefusalAudit_QuietLoginCountSurfaces(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), false)
	step := advancingClock(f)
	for range 3 {
		f.send(commentOpts{body: "/levelrail status"})
	}
	step(defaultPRCommentRefusalAuditWindow)
	f.send(commentOpts{login: "someone-else", body: "/levelrail status"})
	got := f.refusals()
	if len(got) != 3 {
		t.Fatalf("rows = %+v, want first row, summary and the new login's row", got)
	}
	assertSummaryRow(t, got, "octo", 2)
}

// Evicting an expired entry to make room keeps its suppressed count.
func TestPRCommentRefusalAudit_EvictedCountSurfaces(t *testing.T) {
	t.Setenv(envPRCommentRefusalAuditMaxKeys, "1")
	f := newPRCmdFixture(t, testPRCommentConfig(), false)
	step := advancingClock(f)
	for range 4 {
		f.send(commentOpts{body: "/levelrail status"})
	}
	step(defaultPRCommentRefusalAuditWindow)
	f.send(commentOpts{login: "someone-else", body: "/levelrail status"})
	assertSummaryRow(t, f.refusals(), "octo", 3)
}

type failingClaimStore struct{ PRCommentCommandStore }

func (failingClaimStore) ClaimPRCommentCommand(context.Context, string, int64, string, string, int, time.Time, time.Duration) (bool, error) {
	return false, errors.New("injected claim failure")
}

func TestPRCommentRefusalAudit_ClaimFailureLogged(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), false)
	var buf bytes.Buffer
	f.rt.logger = slog.New(slog.NewTextHandler(&buf, nil))
	f.rt.prCommentCommands = failingClaimStore{f.rt.prCommentCommands}
	f.send(commentOpts{body: "/levelrail status"})
	if !strings.Contains(buf.String(), "refusal: claim failed") {
		t.Fatalf("log = %q, want the failed claim logged", buf.String())
	}
	if got := f.refusals(); len(got) != 0 {
		t.Fatalf("rows = %+v, want none without a claim", got)
	}
}
