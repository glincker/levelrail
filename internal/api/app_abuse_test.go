package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/abuse"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

func abuseRouter(t *testing.T) (*Router, *store.DB, *http.Cookie, *ingress.AccessLog) {
	t.Helper()
	db := openTestDB(t)
	stats := ingress.NewRequestStats()
	stats.SetHostOwners(map[string]string{"web.example.com": "web"})
	log := ingress.NewAccessLog(1000)
	rt := NewRouter(discardLogger(), testBrand(), db, WithAccessSource(EdgeAccess{Stats: stats, Log: log}))
	seedApp(t, db, "web")
	return rt, db, loginTestSession(t, rt, db), log
}

func seedAbuseTraffic(log *ingress.AccessLog, now time.Time) {
	for i := 0; i < 40; i++ {
		log.Record("web.example.com", ingress.AccessRecord{
			Time: now.Add(-time.Duration(40-i) * 100 * time.Millisecond), Client: netip.MustParseAddr("192.0.2.50"), UserAgent: "Mozilla", Path: "/",
		})
	}
	log.Record("web.example.com", ingress.AccessRecord{Time: now.Add(-time.Minute), Client: netip.MustParseAddr("203.0.113.9"), UserAgent: "Mozilla"})
	log.Record("web.example.com", ingress.AccessRecord{Time: now.Add(-2 * time.Minute), Client: netip.MustParseAddr("198.51.100.4"), UserAgent: "python-requests/2"})
}

func TestAppAbuse_GetDefaultsToOffWithASuggestion(t *testing.T) {
	t.Parallel()
	rt, _, cookie, _ := abuseRouter(t)
	rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/abuse", "")
	var res abuseResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
		t.Fatalf("GET = %d: %s", rec.Code, rec.Body.String())
	}
	l := abuse.DefaultLimits()
	if res.Configured || res.Policy.Mode != abuse.ModeOff || res.Condition.Reason != abuseReasonOff {
		t.Fatalf("unconfigured app = %+v", res)
	}
	if res.Suggestion.RPS != l.DefaultRPS || res.Suggestion.Burst != l.DefaultBurst || res.Suggestion.FromTraffic {
		t.Fatalf("suggestion without traffic = %+v", res.Suggestion)
	}
	if res.Limits.MaxRPS != l.MaxRPS || len(res.Counters.ByReason) != int(abuse.ReasonCount) {
		t.Fatalf("limits/counters = %+v / %+v", res.Limits, res.Counters)
	}
	if rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/apps/nope/abuse", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown app = %d", rec.Code)
	}
}

func TestAppAbuse_PutRoundTripAndValidation(t *testing.T) {
	t.Parallel()
	rt, db, cookie, _ := abuseRouter(t)
	const path = "/api/v1/apps/web/abuse"
	rec := doPolicy(t, rt, cookie, http.MethodPut, path, `{"mode":"log_only","rps":10,"deny_cidrs":["203.0.113.0/24"],"deny_user_agents":["python-requests"],"require_user_agent":true}`)
	var res abuseResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
		t.Fatalf("PUT = %d: %s", rec.Code, rec.Body.String())
	}
	if !res.Configured || res.Policy.Mode != abuse.ModeLogOnly || res.Policy.Burst == 0 || res.Condition.Reason != abuseReasonPending {
		t.Fatalf("saved policy = %+v", res)
	}
	row, found, err := db.GetAppAbusePolicy(context.Background(), "web")
	if err != nil || !found || row.UpdatedBy == "" {
		t.Fatalf("row = %+v found=%v err=%v", row, found, err)
	}
	if err := db.SetAppAbuseCondition(context.Background(), "web", "True", "LogOnly", "counting"); err != nil {
		t.Fatal(err)
	}
	rec = doPolicy(t, rt, cookie, http.MethodGet, path, "")
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.Condition.Reason != "LogOnly" || res.Condition.Status != "True" {
		t.Fatalf("stored condition not surfaced: %+v", res.Condition)
	}

	bad := []struct{ name, body string }{
		{"nothing on", `{"mode":"enforce"}`},
		{"bad cidr", `{"mode":"enforce","deny_cidrs":["x"]}`},
		{"rps too high", `{"mode":"enforce","rps":999999999}`},
		{"bad mode", `{"mode":"block"}`},
		{"unknown field", `{"mode":"off","nope":1}`},
		{"not json", `{`},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			if rec := doPolicy(t, rt, cookie, http.MethodPut, path, tt.body); rec.Code != http.StatusBadRequest {
				t.Fatalf("PUT = %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
	rec = doPolicy(t, rt, cookie, http.MethodPut, path, `{"mode":"enforce","deny_cidrs":["x"]}`)
	var v abuseValidationResponse
	if json.Unmarshal(rec.Body.Bytes(), &v) != nil || len(v.Fields) == 0 || v.Fields[0].Field != "deny_cidrs[0]" {
		t.Fatalf("validation body = %s", rec.Body.String())
	}
	if rec := doPolicy(t, rt, cookie, http.MethodPut, "/api/v1/apps/nope/abuse", `{"mode":"off"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown app = %d", rec.Code)
	}
}

func TestAppAbuse_RoutesRequireAuth(t *testing.T) {
	t.Parallel()
	rt, _, _, _ := abuseRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/abuse"},
		{http.MethodPut, "/api/v1/apps/web/abuse"},
		{http.MethodPost, "/api/v1/apps/web/abuse/dry-run"},
	})
}

func TestAppAbuse_DryRunCountsWouldBlockWithoutSideEffects(t *testing.T) {
	t.Parallel()
	rt, db, cookie, log := abuseRouter(t)
	seedAbuseTraffic(log, time.Now())
	const path = "/api/v1/apps/web/abuse/dry-run"
	body := `{"minutes":10,"policy":{"mode":"enforce","rps":2,"burst":4,"deny_cidrs":["203.0.113.0/24"],"deny_user_agents":["python-requests"]}}`
	rec := doPolicy(t, rt, cookie, http.MethodPost, path, body)
	var res abuseDryRunResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
		t.Fatalf("dry-run = %d: %s", rec.Code, rec.Body.String())
	}
	if res.Requests != 42 {
		t.Fatalf("Requests = %d, want 42", res.Requests)
	}
	// 40 requests inside 4s at 2 rps with burst 4 admit 4 + 8 = 12, so 28 limited, plus the denied cidr and agent.
	counts := map[string]int{}
	for _, r := range res.ByReason {
		counts[r.Reason] = r.Count
	}
	if counts["deny_cidr"] != 1 || counts["deny_user_agent"] != 1 || counts["rate_limit"] < 20 || counts["rate_limit"] > 30 {
		t.Fatalf("ByReason = %v", counts)
	}
	if len(res.TopClients) == 0 || res.TopClients[0].Client != "192.0.2.50" || res.WindowMinutes != 10 || res.Note == "" {
		t.Fatalf("result = %+v", res)
	}
	if _, found, _ := db.GetAppAbusePolicy(context.Background(), "web"); found {
		t.Fatal("a dry run saved a policy")
	}
	if got := abuse.DefaultRegistry().DrainAll()["web"]; !got.Zero() {
		t.Fatalf("a dry run touched live counters: %+v", got)
	}

	// Mode off in the draft still previews: the point is to see the effect first.
	rec = doPolicy(t, rt, cookie, http.MethodPost, path, `{"policy":{"mode":"off","deny_cidrs":["203.0.113.0/24"]}}`)
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil || res.WouldBlock != 1 {
		t.Fatalf("off-mode draft = %d %s", rec.Code, rec.Body.String())
	}
}

func TestAppAbuse_DryRunUsesSavedPolicyAndRejectsBadInput(t *testing.T) {
	t.Parallel()
	rt, _, cookie, log := abuseRouter(t)
	seedAbuseTraffic(log, time.Now())
	const path = "/api/v1/apps/web/abuse/dry-run"
	if rec := doPolicy(t, rt, cookie, http.MethodPost, path, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("no draft and nothing saved = %d, want 400", rec.Code)
	}
	if rec := doPolicy(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/abuse", `{"mode":"log_only","deny_cidrs":["198.51.100.4"]}`); rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d", rec.Code)
	}
	rec := doPolicy(t, rt, cookie, http.MethodPost, path, "")
	var res abuseDryRunResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil || res.WouldBlock != 1 {
		t.Fatalf("saved-policy dry-run = %d %s", rec.Code, rec.Body.String())
	}
	over := int(abuse.DefaultLimits().DryRunMax.Minutes()) + 1
	for _, body := range []string{`{"minutes":-1,"policy":{"mode":"off"}}`, `{"minutes":` + strconv.Itoa(over) + `,"policy":{"mode":"off"}}`, `{"policy":{"mode":"enforce"}}`} {
		if rec := doPolicy(t, rt, cookie, http.MethodPost, path, body); rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s = %d, want 400", body, rec.Code)
		}
	}
}

func TestAppAbuse_DryRunWithoutAccessLogIs501(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	seedApp(t, db, "web")
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/abuse/dry-run", `{"policy":{"mode":"off"}}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("dry-run without an access log = %d", rec.Code)
	}
}
