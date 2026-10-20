package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDecideHygiene(t *testing.T) {
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	ago := func(days int) *time.Time { v := now.Add(-time.Duration(days) * hygieneDay); return &v }
	grace := 7 * hygieneDay
	tok := func(created int, used *time.Time) store.APIToken {
		return store.APIToken{ID: "t", CreatedAt: *ago(created), LastUsedAt: used}
	}
	notice := func(days int, disabled bool) *store.TokenHygieneNotice {
		n := &store.TokenHygieneNotice{TokenID: "t", NoticedAt: *ago(days)}
		if disabled {
			n.DisabledAt = now
		}
		return n
	}
	tests := []struct {
		name   string
		tok    store.APIToken
		notice *store.TokenHygieneNotice
		days   int
		want   hygieneStep
	}{
		{"policy off", tok(400, nil), nil, 0, hygieneNone},
		{"recently used", tok(400, ago(1)), nil, 90, hygieneNone},
		{"never used but new", tok(10, nil), nil, 90, hygieneNone},
		{"idle gets a notice first", tok(400, ago(100)), nil, 90, hygieneNotice},
		{"inside the grace period", tok(400, ago(100)), notice(3, false), 90, hygieneNone},
		{"grace over disables", tok(400, ago(100)), notice(8, false), 90, hygieneDisable},
		{"used after the notice clears it", tok(400, ago(1)), notice(8, false), 90, hygieneClear},
		{"already disabled", tok(400, ago(100)), notice(30, true), 90, hygieneNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decideHygiene(tt.tok, tt.notice, tt.days, grace, now); got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSweepTokenHygiene_NoticeThenDisable(t *testing.T) {
	h := newCodeHarness(t, false)
	ctx := context.Background()
	admin := adminSessionForTest(t, h)
	rec := h.send(http.MethodPost, "/api/v1/auth/tokens", `{"name":"idle","abilities":["read"]}`, "", admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("mint = %d %s", rec.Code, rec.Body.String())
	}
	var minted createTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &minted); err != nil {
		t.Fatal(err)
	}
	days := 1
	if err := h.db.SaveSecurityPolicy(ctx, store.SecurityPolicy{DisableUnusedDays: &days, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envTokenHygieneGrace, "48h")
	start := time.Now().Add(2 * hygieneDay)
	if err := h.rt.SweepTokenHygiene(ctx, start); err != nil {
		t.Fatal(err)
	}
	notices, _ := h.db.ListTokenHygieneNotices(ctx)
	if len(notices) != 1 || notices[0].TokenID != minted.ID {
		t.Fatalf("notices = %+v", notices)
	}
	if err := h.rt.SweepTokenHygiene(ctx, start.Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	h.rt.sec.hygiene = hygieneClock{}
	if err := h.rt.SweepTokenHygiene(ctx, start.Add(3*hygieneDay)); err != nil {
		t.Fatal(err)
	}
	got, err := h.db.GetAPITokenByID(ctx, minted.ID)
	if err != nil || got.RevokedAt == nil {
		t.Fatalf("token not disabled after grace: %+v %v", got, err)
	}
	actions := h.auditActions()
	if !containsAction(actions, store.AuditActionTokenUnusedNotice) || !containsAction(actions, store.AuditActionTokenUnusedDisabled) {
		t.Fatalf("missing audit rows: %v", actions)
	}
}

func TestFailedLoginCounter_Thresholds(t *testing.T) {
	t.Setenv(envFailedLoginAccountThreshold, "3")
	t.Setenv(envFailedLoginIPThreshold, "5")
	t.Setenv(envFailedLoginWindow, "10m")
	c := newFailedLoginCounter()
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	var raised []loginAnomaly
	for i := range 6 {
		raised = append(raised, c.record("Victim@Example.com", "203.0.113.9", now.Add(time.Duration(i)*time.Second))...)
	}
	tests := []struct {
		kind  string
		count int
	}{{anomalyKindAccount, 3}, {anomalyKindIP, 5}}
	if len(raised) != len(tests) {
		t.Fatalf("raised %+v, want one per key", raised)
	}
	for i, tt := range tests {
		if raised[i].Kind != tt.kind || raised[i].Count != tt.count {
			t.Fatalf("raised[%d] = %+v, want %s at %d", i, raised[i], tt.kind, tt.count)
		}
	}
	if raised[0].Subject != "victim@example.com" {
		t.Fatalf("account key not normalised: %q", raised[0].Subject)
	}
	later := now.Add(11 * time.Minute)
	if got := c.record("victim@example.com", "", later); len(got) != 0 {
		t.Fatalf("a single failure after the window must not alert: %+v", got)
	}
	if got := c.recent(later); len(got) != 2 {
		t.Fatalf("recent = %+v", got)
	}
}

func TestSecurityPosture_SummaryForReadersFullForAdmins(t *testing.T) {
	h := newCodeHarness(t, false)
	tests := []struct {
		name   string
		caller *http.Cookie
		full   bool
	}{
		{"non-admin gets the summary", h.userSession(), false},
		{"admin gets every item", adminSessionForTest(t, h), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := h.send(http.MethodGet, "/api/v1/security/posture", "", "", tt.caller)
			var out postureResponse
			if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
				t.Fatalf("posture = %d %s", rec.Code, rec.Body.String())
			}
			if out.Full != tt.full || (len(out.Items) > 0) != tt.full || len(out.Account) == 0 {
				t.Fatalf("full=%v items=%d account=%d", out.Full, len(out.Items), len(out.Account))
			}
			if out.Score < 0 || out.Score > 100 || out.Grade == "" {
				t.Fatalf("bad score %d %q", out.Score, out.Grade)
			}
		})
	}
}
