package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/rollback"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

func TestRollbackOneClick(t *testing.T) {
	rt, _, _ := newSelfUpgradeRouter(t, true, nil)
	good := rollbackPlanResource{Plan: rollback.Plan{Verdict: upgrade.VerdictBinaryOnly, Target: rollback.Target{Retained: true}}}
	tests := []struct {
		name   string
		mutate func(*rollbackPlanResource)
		want   bool
		why    string
	}{
		{"binary only and retained", func(*rollbackPlanResource) {}, true, ""},
		{"preflight failed", func(p *rollbackPlanResource) { p.Blocked = true }, false, "preflight"},
		{"needs restore", func(p *rollbackPlanResource) { p.RestoreRequired = true; p.Verdict = upgrade.VerdictRestoreRequired }, false, "restore"},
		{"unknown schema", func(p *rollbackPlanResource) { p.Verdict = upgrade.VerdictUnknown }, false, "schema"},
		{"not retained", func(p *rollbackPlanResource) { p.Target.Retained = false }, false, "not kept"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := good
			tt.mutate(&p)
			ok, why := rt.rollbackOneClick(p)
			if ok != tt.want || (tt.why != "" && !strings.Contains(why, tt.why)) {
				t.Fatalf("rollbackOneClick = %v %q, want %v containing %q", ok, why, tt.want, tt.why)
			}
		})
	}

	rt.selfUpgrade.canLaunch = func() (bool, string) { return false, "not root" }
	if ok, why := rt.rollbackOneClick(good); ok || why != "not root" {
		t.Fatalf("host that cannot launch: got %v %q", ok, why)
	}
}

func TestHandleRollbackStartRejectsBadInput(t *testing.T) {
	rt, cookie, launched := newSelfUpgradeRouter(t, true, nil)
	tests := []struct {
		name, body string
		want       int
	}{
		{"invalid json", `{`, http.StatusBadRequest},
		{"invalid version", `{"version":"nope","confirm":"nope"}`, http.StatusBadRequest},
		{"unknown release", `{"version":"v0.0.1","confirm":"v0.0.1"}`, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, body := suDo(t, rt, cookie, http.MethodPost, "/api/v1/updates/rollback", tt.body)
			if code != tt.want {
				t.Fatalf("status %d, want %d: %s", code, tt.want, body)
			}
		})
	}
	if len(*launched) != 0 {
		t.Fatalf("rejected requests launched %d host commands", len(*launched))
	}
}
