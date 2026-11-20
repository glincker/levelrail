package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestAutoUpdateStatusStates(t *testing.T) {
	rt, cookie, _, _ := newScheduleRouter(t)
	put := func(mode string) upgradePolicyResource {
		t.Helper()
		code, body := suDo(t, rt, cookie, http.MethodPut, "/api/v1/updates/auto",
			`{"mode":"`+mode+`","window_cron":"0 3 * * *","window_duration_minutes":90,"timezone":"UTC","min_release_age_days":0,"paused":false}`)
		if code != http.StatusOK {
			t.Fatalf("status %d: %s", code, body)
		}
		return decodeBody[upgradePolicyResource](t, body)
	}
	get := func() *upgradeAutoStatus {
		t.Helper()
		_, body := suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/auto", "")
		return decodeBody[upgradePolicyResource](t, body).Status
	}

	if st := put("off").Status; st == nil || st.State != autoStateOff {
		t.Fatalf("off policy must report off: %+v", st)
	}

	put("patch")
	if st := get(); st == nil || st.State != autoStateReady || !st.CanApply {
		t.Fatalf("a healthy host must be ready: %+v", st)
	}

	// A host that cannot launch an upgrade is blocked, with the reason named.
	rt.selfUpgrade.canLaunch = func() (bool, string) { return false, "no systemd" }
	if st := get(); st == nil || st.State != autoStateBlocked || st.CanApply || st.Reason != "no systemd" {
		t.Fatalf("a host that cannot apply must be blocked with its reason: %+v", st)
	}
	rt.selfUpgrade.canLaunch = func() (bool, string) { return true, "" }

	// Failed release lookups degrade the state and carry the failure count.
	rt.fetchLatestRelease = func(context.Context) (*githubRelease, error) {
		return nil, errors.New("github unreachable")
	}
	if err := rt.UpgradeScheduler().Tick(context.Background()); err != nil {
		t.Logf("tick: %v", err)
	}
	st := get()
	if st == nil || st.State != autoStateDegraded || st.CheckFailures < 1 || st.Reason == "" || st.NextCheckAt == "" {
		t.Fatalf("failed checks must degrade with a retry time: %+v", st)
	}

	// Without a control plane backup, automatic updates are held, so status must say blocked.
	rt.cpBackups = nil
	st = get()
	if st == nil || st.State != autoStateBlocked || st.CanApply || !strings.Contains(st.Reason, "no control plane backup") {
		t.Fatalf("a host without a backup must be blocked with a reason: %+v", st)
	}
}
