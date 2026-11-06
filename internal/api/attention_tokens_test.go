package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// setTokenTimes moves a token's expiry and last use in the library and mirror tables.
func setTokenTimes(t *testing.T, db *store.DB, name string, expires time.Time, lastUsed *time.Time) {
	t.Helper()
	ctx := context.Background()
	var mirrorUsed, libUsed any
	if lastUsed != nil {
		mirrorUsed, libUsed = lastUsed.UTC().Format(time.RFC3339Nano), lastUsed.UnixMicro()
	}
	if _, err := db.ExecContext(ctx, `UPDATE api_tokens SET expires_at = ?, last_used_at = ? WHERE name = ?`, expires.UTC().Format(time.RFC3339Nano), mirrorUsed, name); err != nil {
		t.Fatalf("update mirror: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE theauth_api_tokens SET expires_at = ?, last_used_at = ? WHERE name = ?`, expires.UnixMicro(), libUsed, name); err != nil {
		t.Fatalf("update library: %v", err)
	}
}

func rowsOfKind(items []attention.Item, kind string) []attention.Item {
	var out []attention.Item
	for _, it := range items {
		if it.Kind == kind {
			out = append(out, it)
		}
	}
	return out
}

func TestAttentionFeed_ExpiredTokensAreOneRow(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	for _, n := range []string{"emergency-a", "emergency-b", "emergency-c"} {
		mintExpiringToken(t, rt, admin, n, 30)
		setTokenTimes(t, db, n, time.Now().Add(-48*time.Hour), nil)
	}
	items := fetchFeed(t, rt, authedRequest(t, admin, "GET", "/api/v1/attention/feed", ""))
	rows := rowsOfKind(items, attention.KindTokenExpired)
	if len(rows) != 1 {
		t.Fatalf("expired rows = %d, want one grouped row: %+v", len(rows), rows)
	}
	if rows[0].Severity != attention.Info || rows[0].Params["count"] != "3" {
		t.Errorf("row = %+v, want an info row counting 3", rows[0])
	}
	if got := rowsOfKind(items, attention.KindTokenExpiredInUse); len(got) != 0 {
		t.Errorf("unused tokens flagged as in use: %+v", got)
	}
}

func TestAttentionFeed_UsedExpiredTokenIsFlagged(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	mintExpiringToken(t, rt, admin, "busy", 30)
	mintExpiringToken(t, rt, admin, "idle", 30)
	used := time.Now().Add(-49 * time.Hour)
	setTokenTimes(t, db, "busy", time.Now().Add(-48*time.Hour), &used)
	setTokenTimes(t, db, "idle", time.Now().Add(-48*time.Hour), nil)
	items := fetchFeed(t, rt, authedRequest(t, admin, "GET", "/api/v1/attention/feed", ""))
	inUse := rowsOfKind(items, attention.KindTokenExpiredInUse)
	if len(inUse) != 1 || inUse[0].Severity != attention.Warning || inUse[0].Params["names"] != "busy" {
		t.Fatalf("in-use rows = %+v, want one warning naming busy", inUse)
	}
	if !strings.Contains(inUse[0].Title, "used within the last 7 days") || !strings.Contains(inUse[0].Detail, "used within the last 7 days") {
		t.Errorf("wording = %q / %q, want last-used wording", inUse[0].Title, inUse[0].Detail)
	}
	if idle := rowsOfKind(items, attention.KindTokenExpired); len(idle) != 1 || idle[0].Params["names"] != "idle" {
		t.Errorf("idle rows = %+v, want one naming idle", idle)
	}
}

func TestAttentionFeed_PlatformTokenHiddenAndRotatedBeforeExpiry(t *testing.T) {
	t.Setenv(envStackDriftTTL, "1h")
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	mintExpiringToken(t, rt, admin, "human-ci", 2)
	ctx := context.Background()

	if _, err := rt.driftToken(ctx); err != nil {
		t.Fatalf("mint drift token: %v", err)
	}
	firstID := rt.stackDrift.id
	items := fetchFeed(t, rt, authedRequest(t, admin, "GET", "/api/v1/attention/feed", ""))
	for _, n := range subjectsOfKind(items, attention.KindTokenExpiring) {
		if n == stackDriftTokenName {
			t.Fatalf("platform token listed as expiring: %v", n)
		}
	}

	rt.stackDrift.expires = time.Now().Add(time.Minute)
	if _, err := rt.driftToken(ctx); err != nil {
		t.Fatalf("rotate drift token: %v", err)
	}
	if rt.stackDrift.id == firstID {
		t.Fatal("drift token was not rotated inside the lead window")
	}
	toks, err := rt.tokens.ListAPITokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range toks {
		switch tk.ID {
		case firstID:
			if tk.RevokedAt == nil {
				t.Error("old drift token was not revoked")
			}
		case rt.stackDrift.id:
			if tk.RevokedAt != nil {
				t.Error("new drift token is revoked")
			}
		}
		if tk.Name == "human-ci" && tk.RevokedAt != nil {
			t.Error("a token a human created was revoked by rotation")
		}
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log WHERE action = ?`, auditActionTokenRotated).Scan(&n); err != nil || n != 1 {
		t.Errorf("rotation audit rows = %d (err %v), want 1", n, err)
	}
	if got := subjectsOfKind(fetchFeed(t, rt, authedRequest(t, admin, "GET", "/api/v1/attention/feed", "")), attention.KindTokenExpiring); len(got) != 1 || got[0] != "human-ci" {
		t.Errorf("expiring names = %v, want only the human token", got)
	}
}

func TestTokenSettingsRanges(t *testing.T) {
	checks := map[string][]struct {
		val string
		ok  bool
	}{
		runtimesettings.KeyTokensExpiryWarningDays:      {{"0", false}, {"1", true}, {"7", true}, {"90", true}, {"91", false}},
		runtimesettings.KeyTokensSystemRotationLeadMins: {{"9", false}, {"10", true}, {"1440", true}, {"1441", false}},
	}
	for _, d := range runtimesettings.Definitions() {
		cases, ok := checks[d.Key]
		if !ok {
			continue
		}
		if d.CheckValue == nil {
			t.Fatalf("%s has no range check", d.Key)
		}
		for _, c := range cases {
			if err := d.CheckValue(c.val); (err == nil) != c.ok {
				t.Errorf("%s %s: err = %v, want ok=%v", d.Key, c.val, err, c.ok)
			}
		}
		delete(checks, d.Key)
	}
	if len(checks) != 0 {
		t.Errorf("settings not registered: %v", checks)
	}
}

func TestAttentionFeed_HumanTokenWithSystemScopeStillShows(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	mintExpiringToken(t, rt, admin, "sneaky", 2)
	var id string
	if err := db.QueryRowContext(context.Background(), `SELECT id FROM api_tokens WHERE name = 'sneaky'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAPITokenScope(context.Background(), store.APITokenScope{TokenID: id, TeamID: systemTokenTeamID, Role: store.TeamRoleOwner}); err != nil {
		t.Fatal(err)
	}
	items := fetchFeed(t, rt, authedRequest(t, admin, http.MethodGet, "/api/v1/attention/feed", ""))
	if got := subjectsOfKind(items, attention.KindTokenExpiring); len(got) != 1 || got[0] != "sneaky" {
		t.Errorf("expiring = %v, want the owned token despite its system scope", got)
	}
}

func TestCreateToken_RejectsSystemTeam(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	body := `{"name":"x","abilities":["read"],"team_id":"system"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, admin, http.MethodPost, "/api/v1/auth/tokens", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("create with team_id system = %d, want 400", rec.Code)
	}
}

func TestSystemRotationLeadFloor(t *testing.T) {
	ttl := 2 * time.Hour
	for _, tc := range []struct{ setting, want time.Duration }{
		{10 * time.Minute, 15 * time.Minute},
		{60 * time.Minute, 60 * time.Minute},
		{1000 * time.Minute, time.Hour},
	} {
		if got := effectiveRotationLead(tc.setting, ttl); got != tc.want {
			t.Errorf("lead(%v) = %v, want %v", tc.setting, got, tc.want)
		}
	}
}

func TestDriftTokenRotationAuditNotesFailedRevoke(t *testing.T) {
	t.Setenv(envStackDriftTTL, "1h")
	rt, db := newTestRouter(t)
	ctx := context.Background()
	rt.stackDrift.raw, rt.stackDrift.id, rt.stackDrift.expires = "stale", "missing-id", time.Now().Add(time.Minute)
	if _, err := rt.driftToken(ctx); err != nil {
		t.Fatal(err)
	}
	var detail string
	if err := db.QueryRowContext(ctx, `SELECT detail FROM audit_log WHERE action = ?`, auditActionTokenRotated).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail, `"old_revoked":false`) {
		t.Errorf("detail = %s, want old_revoked false", detail)
	}
}
