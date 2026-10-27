package api

import (
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
