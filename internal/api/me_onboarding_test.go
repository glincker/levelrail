package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/setupguide"
	"github.com/GLINCKER/levelrail/internal/store"
)

func decodeMeOnboarding(t *testing.T, body string) meOnboardingResource {
	t.Helper()
	var out meOnboardingResource
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return out
}

func TestMeOnboarding_DefaultTransitionsAndReset(t *testing.T) {
	h := newCodeHarness(t, false)
	own := h.userSession()

	rec := h.send(http.MethodGet, "/api/v1/me/onboarding", "", "", own)
	if rec.Code != http.StatusOK {
		t.Fatalf("get = %d %s", rec.Code, rec.Body.String())
	}
	got := decodeMeOnboarding(t, rec.Body.String())
	if got.State != "not_started" || got.CurrentStep != "welcome" || got.FlowVersion != 1 || !got.Enabled || got.StartedAt != nil {
		t.Fatalf("default = %+v", got)
	}

	rec = h.send(http.MethodPut, "/api/v1/me/onboarding", `{"state":"in_progress","current_step":"deploy"}`, "", own)
	if rec.Code != http.StatusOK {
		t.Fatalf("put = %d %s", rec.Code, rec.Body.String())
	}
	got = decodeMeOnboarding(t, rec.Body.String())
	if got.State != "in_progress" || got.CurrentStep != "deploy" || got.StartedAt == nil {
		t.Fatalf("after start = %+v", got)
	}

	rec = h.send(http.MethodPut, "/api/v1/me/onboarding", `{"state":"not_started"}`, "", own)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"invalid_transition"`) {
		t.Fatalf("illegal move = %d %s", rec.Code, rec.Body.String())
	}
	rec = h.send(http.MethodPut, "/api/v1/me/onboarding", `{"state":"bogus"}`, "", own)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown state = %d", rec.Code)
	}

	rec = h.send(http.MethodPost, "/api/v1/me/onboarding/reset", "", "", own)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset = %d %s", rec.Code, rec.Body.String())
	}
	if got = decodeMeOnboarding(t, rec.Body.String()); got.State != "not_started" || got.StartedAt != nil {
		t.Fatalf("after reset = %+v", got)
	}
}

func TestMeOnboarding_MembersOnlyTouchTheirOwnRow(t *testing.T) {
	h := newCodeHarness(t, false)
	own := h.userSession()
	other := accountOtherUser(t, h)

	h.send(http.MethodPut, "/api/v1/me/onboarding", `{"state":"in_progress"}`, "", own)
	// A user id in the body or query must change nothing.
	h.send(http.MethodPut, "/api/v1/me/onboarding?user_id=user_other", `{"state":"dismissed","user_id":"user_other"}`, "", own)

	rec := h.send(http.MethodGet, "/api/v1/me/onboarding", "", "", other)
	if got := decodeMeOnboarding(t, rec.Body.String()); got.State != "not_started" {
		t.Fatalf("other user state = %q, want untouched", got.State)
	}
	if rec := h.send(http.MethodGet, "/api/v1/me/onboarding", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", rec.Code)
	}
}

func TestMeOnboarding_TokenWithoutUserIsRefused(t *testing.T) {
	h := newCodeHarness(t, false)
	plain := seedOwnedToken(t, h, "tok_nouser", "", []string{AbilityRead, AbilityWrite})
	rec := serve(h.rt, bearerRequest(http.MethodGet, "/api/v1/me/onboarding", "", plain))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "not tied to a user") {
		t.Fatalf("ownerless token = %d %s", rec.Code, rec.Body.String())
	}
}

func TestMeOnboarding_DisabledFlagAllowsOnlyDismiss(t *testing.T) {
	h := newCodeHarness(t, false)
	own := h.userSession()
	svc := runtimesettings.New(h.db, func(string) (string, bool) { return "", false })
	if err := h.db.SetRuntimeSetting(t.Context(), runtimesettings.KeyOnboardingEnabled, "false", "test", time.Now()); err != nil {
		t.Fatal(err)
	}
	h.rt.runtimeSettings = svc

	rec := h.send(http.MethodGet, "/api/v1/me/onboarding", "", "", own)
	if got := decodeMeOnboarding(t, rec.Body.String()); got.Enabled {
		t.Fatalf("enabled = true, want false")
	}
	if rec := h.send(http.MethodPut, "/api/v1/me/onboarding", `{"state":"in_progress"}`, "", own); rec.Code != http.StatusForbidden {
		t.Fatalf("start while disabled = %d", rec.Code)
	}
	if rec := h.send(http.MethodPut, "/api/v1/me/onboarding", `{"state":"dismissed"}`, "", own); rec.Code != http.StatusOK {
		t.Fatalf("dismiss while disabled = %d", rec.Code)
	}
}

func TestMeOnboarding_AuditsWritesAndCarriesMilestone(t *testing.T) {
	h := newCodeHarness(t, false)
	own := h.userSession()
	h.send(http.MethodPut, "/api/v1/me/onboarding", `{"state":"in_progress"}`, "", own)
	h.send(http.MethodPost, "/api/v1/me/onboarding/reset", "", "", own)

	entries, err := h.db.ListAuditEntries(t.Context(), 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Action] = true
	}
	if !seen[onboardingEventUpdated] || !seen[onboardingEventReset] {
		t.Fatalf("audit actions = %v, want updated and reset", seen)
	}

	rec := h.send(http.MethodGet, "/api/v1/me/onboarding", "", "", own)
	if got := decodeMeOnboarding(t, rec.Body.String()); got.Milestone == "" || got.NextMilestone == "" {
		t.Fatalf("milestone = %q next = %q, want the setup guide milestone merged in", got.Milestone, got.NextMilestone)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"state", "current_step", "flow_version", "enabled", "started_at", "completed_at", "dismissed_at", "milestone", "next_milestone"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("response lacks %q: %s", k, rec.Body.String())
		}
	}
}

func TestMeOnboarding_RestoreReminderDue(t *testing.T) {
	h := newCodeHarness(t, false)
	h.rt.runtimeSettings = runtimesettings.New(h.db, func(string) (string, bool) { return "", false })
	r := httptest.NewRequest(http.MethodGet, "/api/v1/me/onboarding", nil)
	old := time.Now().Add(-8 * 24 * time.Hour)
	fresh := time.Now().Add(-2 * 24 * time.Hour)
	withBackup := setupguide.Input{ControlPlaneBackups: 1}
	cases := []struct {
		name    string
		started *time.Time
		in      setupguide.Input
		want    bool
	}{
		{"past window with unverified backup", &old, withBackup, true},
		{"inside window", &fresh, withBackup, false},
		{"restore verified", &old, setupguide.Input{ControlPlaneBackups: 1, VerifiedBackups: 1}, false},
		{"no backup", &old, setupguide.Input{}, false},
		{"never started", nil, withBackup, false},
	}
	for _, c := range cases {
		if got := h.rt.restoreReminderDue(r, store.UserOnboarding{StartedAt: c.started}, c.in); got != c.want {
			t.Errorf("%s: due = %v, want %v", c.name, got, c.want)
		}
	}
	if err := h.db.SetRuntimeSetting(t.Context(), runtimesettings.KeyOnboardingRestoreReminderDays, "0", "test", time.Now()); err != nil {
		t.Fatal(err)
	}
	h.rt.runtimeSettings = runtimesettings.New(h.db, func(string) (string, bool) { return "", false })
	if h.rt.restoreReminderDue(r, store.UserOnboarding{StartedAt: &old}, withBackup) {
		t.Error("days=0 must turn the reminder off")
	}
}
