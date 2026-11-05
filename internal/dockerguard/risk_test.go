package dockerguard

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"
)

func TestEveryRuleHasARiskClass(t *testing.T) {
	t.Parallel()
	low := map[string]bool{}
	for _, r := range AllRules {
		class, ok := ruleRisk[r]
		if !ok {
			t.Errorf("rule %q has no risk class", r)
		}
		if class == RiskLow {
			low[r] = true
		}
		if (class == RiskHigh) != RuleIsHighRisk(r) {
			t.Errorf("rule %q: RuleIsHighRisk disagrees with its class", r)
		}
	}
	if len(low) != 2 || !low[RuleBindUndeclared] || !low[RuleMountType] {
		t.Errorf("low risk rules = %v, want only bind_undeclared and mount_type_disallowed", low)
	}
	if !RuleIsHighRisk("rule_added_in_the_future") {
		t.Error("an unknown rule must default to high risk")
	}
}

func TestDecisionRiskRuleLaterHighWins(t *testing.T) {
	t.Parallel()
	d := Decision{Violations: []Violation{{Rule: RuleBindUndeclared}, {Rule: RulePrivileged}, {Rule: RuleBindSensitive}}}
	if d.RiskRule() != RulePrivileged || !d.HighRisk() {
		t.Fatalf("risk rule = %q high=%v", d.RiskRule(), d.HighRisk())
	}
	low := Decision{Violations: []Violation{{Rule: RuleBindUndeclared}, {Rule: RuleMountType}}}
	if low.RiskRule() != RuleBindUndeclared || low.HighRisk() {
		t.Fatalf("low decision = %q high=%v", low.RiskRule(), low.HighRisk())
	}
}

func TestDedupDoesNotDropAHighRiskDecisionBehindALowOne(t *testing.T) {
	t.Parallel()
	r := newRecorder(nil, Tunables{AuditDedup: time.Hour}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	at := time.Now()
	lowFirst := Decision{Method: "POST", Pattern: "/containers/create", At: at, Violations: []Violation{{Rule: RuleBindUndeclared}}}
	highSamePrimary := Decision{Method: "POST", Pattern: "/containers/create", At: at.Add(time.Minute), Violations: []Violation{{Rule: RuleBindUndeclared}, {Rule: RuleVolumeBindDriverOpt}}}
	if r.duplicate(lowFirst) || r.duplicate(highSamePrimary) {
		t.Fatal("the high-risk decision was dropped as a duplicate of the low-risk one")
	}
	if !r.duplicate(highSamePrimary) {
		t.Fatal("an identical high-risk decision should still dedupe")
	}
}

func TestAuditSinceFollowsWhatActuallyRan(t *testing.T) {
	march := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	boot := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	newCtl := func(t *testing.T, s Settings, srv *Server) *Controller {
		path := filepath.Join(t.TempDir(), SettingsFile)
		if err := SaveSettings(path, s); err != nil {
			t.Fatal(err)
		}
		return NewController(path, envMap(nil), srv, "", nil)
	}
	t.Run("pinned off for months then audit boot starts a fresh window", func(t *testing.T) {
		h := startGuard(t, ModeAudit)
		c := newCtl(t, Settings{AuditSince: march, LastRunMode: ModeOff}, h.server)
		if err := c.NoteBoot(boot); err != nil {
			t.Fatal(err)
		}
		if got := c.Status().AuditSince; !got.Equal(boot) {
			t.Fatalf("AuditSince = %v, want boot time", got)
		}
	})
	t.Run("audit to audit restart keeps the window", func(t *testing.T) {
		h := startGuard(t, ModeAudit)
		c := newCtl(t, Settings{AuditSince: march, LastRunMode: ModeAudit}, h.server)
		if err := c.NoteBoot(boot); err != nil {
			t.Fatal(err)
		}
		if got := c.Status().AuditSince; !got.Equal(march) {
			t.Fatalf("AuditSince = %v, want kept", got)
		}
	})
	t.Run("guard not running clears the window", func(t *testing.T) {
		c := newCtl(t, Settings{AuditSince: march, LastRunMode: ModeAudit}, nil)
		if err := c.NoteBoot(boot); err != nil {
			t.Fatal(err)
		}
		if got := c.Status().AuditSince; !got.IsZero() {
			t.Fatalf("a failed or off start must clear AuditSince, got %v", got)
		}
	})
	t.Run("set audit while not running stamps nothing", func(t *testing.T) {
		c := newCtl(t, Settings{Mode: ModeOff, LastRunMode: ModeOff}, nil)
		if st, err := c.SetMode(ModeAudit, "admin", boot); err != nil || !st.AuditSince.IsZero() {
			t.Fatalf("status = %+v, %v", st, err)
		}
	})
	t.Run("live switch into audit restarts the window", func(t *testing.T) {
		h := startGuard(t, ModeEnforce)
		c := newCtl(t, Settings{AuditSince: march, LastRunMode: ModeEnforce}, h.server)
		if st, _ := c.SetMode(ModeAudit, "admin", boot); !st.AuditSince.Equal(boot) {
			t.Fatalf("AuditSince = %v, want %v", st.AuditSince, boot)
		}
		if st, _ := c.SetMode(ModeAudit, "admin", boot.Add(time.Hour)); !st.AuditSince.Equal(boot) {
			t.Fatalf("audit to audit must keep the window, got %v", st.AuditSince)
		}
	})
}
