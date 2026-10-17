package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
)

type fakeTrustControl struct {
	err    error
	asked  []string
	events []store.NodeSecurityEvent
}

func (f *fakeTrustControl) RequestRenew(id string) error {
	f.asked = append(f.asked, id)
	return f.err
}

func (f *fakeTrustControl) RecordSecurityEvent(ev store.NodeSecurityEvent) {
	f.events = append(f.events, ev)
}

func trustReq(t *testing.T, rt *Router, cookie *http.Cookie, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, ""))
	return rec
}

func TestNodeTrust_LocalNodeHasNoCertificateAndShowsControlPlaneBuild(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.SetLocalNodeID("local")
	cookie := loginTestSession(t, rt, db)
	seedCertNode(t, db, "local", time.Hour, "")

	got := getNodeResource(t, rt, cookie, "local")
	if !got.IsLocal || got.Cert != nil {
		t.Fatalf("local node: is_local=%v cert=%+v, want no certificate resource", got.IsLocal, got.Cert)
	}
	if got.Agent == nil || got.Agent.Version != version.Version || got.Agent.OS != runtime.GOOS || got.Agent.Arch != runtime.GOARCH {
		t.Fatalf("local agent = %+v, want the control plane's own version and platform", got.Agent)
	}

	var trust nodeTrustResource
	rec := trustReq(t, rt, cookie, http.MethodGet, "/api/v1/nodes/local/security")
	if err := json.Unmarshal(rec.Body.Bytes(), &trust); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET security = %d %s (%v)", rec.Code, rec.Body.String(), err)
	}
	if !trust.IsLocal || trust.Status.State != trustLocal || trust.Cert != nil || trust.CA != nil || trust.Renewal != nil {
		t.Fatalf("local trust = %+v, want only the local status", trust)
	}
	for _, target := range []string{"/api/v1/nodes/local/cert/renew", "/api/v1/nodes/local/revoke-cert", "/api/v1/nodes/local/reenroll-token"} {
		if rec := trustReq(t, rt, cookie, http.MethodPost, target); rec.Code != http.StatusBadRequest {
			t.Errorf("POST %s = %d, want 400 for an in-process node", target, rec.Code)
		}
	}
}

func TestNodeTrust_SummaryIsComputedFromRealEvents(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.agentCAFingerprint = "abcd"
	cookie := loginTestSession(t, rt, db)
	seedCertNode(t, db, "n1", 30*24*time.Hour, "v1.0.0")
	ctx := context.Background()
	now := time.Now()
	pol := store.SecurityEventPolicy{Coalesce: true, Window: time.Hour, PerNodeCap: 100}
	add := func(ev store.NodeSecurityEvent) {
		t.Helper()
		if err := db.AddNodeSecurityEvent(ctx, ev, pol, now); err != nil {
			t.Fatalf("AddNodeSecurityEvent() error = %v", err)
		}
	}

	if got := trustFor(t, rt, cookie); got.Status.State != trustNoActivity {
		t.Fatalf("no events: status = %+v, want no_activity (never a hardcoded success)", got.Status)
	}

	add(store.NodeSecurityEvent{NodeID: "n1", Kind: store.SecurityKindConnect, Outcome: store.SecurityOutcomeAccepted, Reason: store.SecurityReasonVerified, SourceIP: "10.0.0.2", Detail: store.NodeSecurityDetail{Fingerprint: "fp-n1"}})
	add(store.NodeSecurityEvent{NodeID: "n1", Kind: store.SecurityKindConnect, Outcome: store.SecurityOutcomeRejected, Reason: store.SecurityReasonExpired, SourceIP: "203.0.113.5", Count: 3})
	got := trustFor(t, rt, cookie)
	if got.Status.State != trustVerified {
		t.Fatalf("online node with an accepted matching connection: status = %+v, want verified despite a stray rejection", got.Status)
	}
	if v := got.Validation; v == nil || v.Accepted != 1 || v.Rejected != 3 {
		t.Fatalf("validation = %+v, want 1 accepted and 3 rejected", v)
	}
	if got.CA == nil || got.CA.Fingerprint != "abcd" || got.Cert == nil || got.Cert.Fingerprint != "fp-n1" || got.Cert.DaysRemaining == nil || got.Cert.LifetimeUsed == nil {
		t.Fatalf("ca/cert = %+v / %+v", got.CA, got.Cert)
	}
	if got.Renewal == nil || got.Renewal.NextRenewAt == nil || !got.Renewal.NextRenewEstimated {
		t.Fatalf("renewal = %+v, want an estimated next renewal until the agent reports its own", got.Renewal)
	}

	if err := db.UpdateNodeStatus(ctx, "n1", store.NodeStatusOffline); err != nil {
		t.Fatal(err)
	}
	if got := trustFor(t, rt, cookie); got.Status.State != trustRejected || got.Status.Reason != store.SecurityReasonExpired {
		t.Fatalf("offline node with a recent rejection: status = %+v, want rejected/expired", got.Status)
	}

	if err := db.RevokeNodeCert(ctx, "n1", now); err != nil {
		t.Fatal(err)
	}
	if got := trustFor(t, rt, cookie); got.Status.State != trustRevoked {
		t.Fatalf("revoked: status = %+v", got.Status)
	}
}

func trustFor(t *testing.T, rt *Router, cookie *http.Cookie) nodeTrustResource {
	t.Helper()
	rec := trustReq(t, rt, cookie, http.MethodGet, "/api/v1/nodes/n1/security")
	var out nodeTrustResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		t.Fatalf("GET security = %d %s", rec.Code, rec.Body.String())
	}
	return out
}

func TestNodeTrust_EventsFilterAndLimit(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedCertNode(t, db, "n1", time.Hour, "")
	pol := store.SecurityEventPolicy{PerNodeCap: 100}
	for _, o := range []string{store.SecurityOutcomeAccepted, store.SecurityOutcomeRejected, store.SecurityOutcomeRejected} {
		if err := db.AddNodeSecurityEvent(context.Background(), store.NodeSecurityEvent{NodeID: "n1", Kind: store.SecurityKindConnect, Outcome: o, Reason: "r"}, pol, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	var evs []nodeSecurityEventResource
	rec := trustReq(t, rt, cookie, http.MethodGet, "/api/v1/nodes/n1/security/events?outcome=rejected")
	if err := json.Unmarshal(rec.Body.Bytes(), &evs); err != nil || len(evs) != 2 {
		t.Fatalf("rejected events = %d (%v) %s, want 2", len(evs), err, rec.Body.String())
	}
	for _, target := range []string{"/api/v1/nodes/n1/security/events?outcome=bogus", "/api/v1/nodes/n1/security/events?limit=0", "/api/v1/nodes/n1/security/events?limit=9999"} {
		if rec := trustReq(t, rt, cookie, http.MethodGet, target); rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", target, rec.Code)
		}
	}
	if rec := trustReq(t, rt, cookie, http.MethodGet, "/api/v1/nodes/missing/security/events"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown node = %d, want 404", rec.Code)
	}
}

func TestNodeTrust_RenewNow(t *testing.T) {
	tests := []struct {
		name    string
		trust   *fakeTrustControl
		revoke  bool
		want    int
		wantEvt int
	}{
		{"connected agent is asked and the action is recorded", &fakeTrustControl{}, false, http.StatusAccepted, 1},
		{"offline node", &fakeTrustControl{err: agent.ErrNodeNotConnected}, false, http.StatusConflict, 0},
		{"revoked certificate points to re-enroll", &fakeTrustControl{}, true, http.StatusConflict, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			WithNodeTrustControl(tt.trust)(rt)
			cookie := loginTestSession(t, rt, db)
			seedCertNode(t, db, "n1", time.Hour, "")
			if tt.revoke {
				_ = db.RevokeNodeCert(context.Background(), "n1", time.Now())
			}
			if rec := trustReq(t, rt, cookie, http.MethodPost, "/api/v1/nodes/n1/cert/renew"); rec.Code != tt.want {
				t.Fatalf("status = %d (%s), want %d", rec.Code, rec.Body.String(), tt.want)
			}
			if len(tt.trust.events) != tt.wantEvt {
				t.Fatalf("recorded events = %+v, want %d", tt.trust.events, tt.wantEvt)
			}
		})
	}
}

func TestNodeTrustRoutes_RootOnly(t *testing.T) {
	rt, db := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/nodes/n1/security"},
		{http.MethodGet, "/api/v1/nodes/n1/security/events"},
		{http.MethodPost, "/api/v1/nodes/n1/cert/renew"},
	})
	bootstrapTestAdmin(t, db)
	seedCertNode(t, db, "n1", time.Hour, "")
	reader := storeUserWithAbilitiesForTest(t, db, "viewer@example.com", []string{AbilityRead})
	cookie := sessionCookieForTest(t, rt, reader.ID)
	for _, r := range []struct{ method, target string }{
		{http.MethodGet, "/api/v1/nodes/n1/security"},
		{http.MethodGet, "/api/v1/nodes/n1/security/events"},
		{http.MethodPost, "/api/v1/nodes/n1/cert/renew"},
	} {
		if rec := trustReq(t, rt, cookie, r.method, r.target); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as a read-only user = %d, want 403", r.method, r.target, rec.Code)
		}
	}
}
