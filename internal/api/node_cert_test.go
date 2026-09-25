package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeSessionCloser struct{ closed []string }

func (f *fakeSessionCloser) Disconnect(id string) { f.closed = append(f.closed, id) }

func seedCertNode(t *testing.T, db *store.DB, id string, left time.Duration, agentVersion string) {
	t.Helper()
	now := time.Now()
	notAfter := now.Add(left)
	if err := db.SaveNode(context.Background(), store.Node{
		ID: id, Name: "name-" + id, Status: store.NodeStatusOnline, CertFingerprint: "fp-" + id, CertNotAfter: &notAfter,
		CertKeyOrigin: store.CertKeyOriginAgent, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if agentVersion != "" {
		if err := db.UpdateNodeAgentInfo(context.Background(), id, store.NodeAgentInfo{Version: agentVersion, OS: "linux", Arch: "amd64"}, now); err != nil {
			t.Fatalf("seed agent info: %v", err)
		}
	}
}

func getNodeResource(t *testing.T, rt *Router, cookie *http.Cookie, id string) nodeResource {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/"+id, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET node status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got nodeResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return got
}

func TestNodeCert_StateAndAgentVersionOnNodeRoutes(t *testing.T) {
	day := 24 * time.Hour
	tests := []struct {
		name         string
		left         time.Duration
		agent        string
		minVersion   string
		wantState    string
		wantDays     int
		wantOutdated bool
	}{
		{"healthy and current", 60*day + time.Hour, "v1.4.0", "v1.2.0", "ok", 60, false},
		{"expiring and outdated", 10*day + time.Hour, "v1.1.9", "v1.2.0", "expiring", 10, true},
		{"critical", 2*day + time.Hour, "v1.2.0", "v1.2.0", "critical", 2, false},
		{"expired, never reported a version", -day - time.Hour, "", "v1.2.0", "expired", -2, true},
		{"no minimum configured", 60*day + time.Hour, "", "", "ok", 60, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			WithMinAgentVersion(tt.minVersion)(rt)
			cookie := loginTestSession(t, rt, db)
			seedCertNode(t, db, "n1", tt.left, tt.agent)

			got := getNodeResource(t, rt, cookie, "n1")
			if got.Cert == nil || got.Agent == nil {
				t.Fatalf("cert/agent missing: %+v", got)
			}
			if got.Cert.State != tt.wantState || got.Cert.DaysRemaining == nil || *got.Cert.DaysRemaining != tt.wantDays {
				t.Errorf("cert = %+v (days %v), want state %q days %d", got.Cert, got.Cert.DaysRemaining, tt.wantState, tt.wantDays)
			}
			if got.Agent.Outdated != tt.wantOutdated {
				t.Errorf("agent.outdated = %v, want %v", got.Agent.Outdated, tt.wantOutdated)
			}

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes", ""))
			var list []nodeResource
			if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0].Cert == nil || list[0].Cert.State != tt.wantState {
				t.Fatalf("list cert state = %+v, err %v", list, err)
			}
		})
	}
}

func TestHandleCreateNodeReenrollToken(t *testing.T) {
	rt, db := newTestRouter(t)
	WithAgentCAFingerprint("abc123")(rt)
	cookie := loginTestSession(t, rt, db)
	seedCertNode(t, db, "n1", time.Hour, "")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/n1/reenroll-token", ""))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got reenrollTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Token == "" || got.NodeID != "n1" || got.CAFingerprint != "abc123" || !got.ExpiresAt.After(time.Now()) {
		t.Fatalf("response = %+v", got)
	}
	stored, err := db.GetNodeJoinTokenByHash(context.Background(), hashToken(got.Token))
	if err != nil {
		t.Fatalf("GetNodeJoinTokenByHash() error = %v", err)
	}
	if stored.Purpose != store.NodeJoinTokenPurposeReenroll || stored.NodeID != "n1" {
		t.Errorf("stored token = %+v, want a reenroll token bound to n1", stored)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/missing/reenroll-token", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown node status = %d, want 404", rec.Code)
	}
}

func TestHandleRevokeNodeCert(t *testing.T) {
	rt, db := newTestRouter(t)
	closer := &fakeSessionCloser{}
	WithNodeSessionCloser(closer)(rt)
	cookie := loginTestSession(t, rt, db)
	seedCertNode(t, db, "n1", 30*24*time.Hour, "")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/n1/revoke-cert", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got nodeResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Cert == nil || got.Cert.State != "revoked" || got.Cert.RevokedAt == nil {
		t.Fatalf("cert after revoke = %+v", got.Cert)
	}
	if len(closer.closed) != 1 || closer.closed[0] != "n1" {
		t.Errorf("live session closed for %v, want [n1]", closer.closed)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/missing/revoke-cert", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown node status = %d, want 404", rec.Code)
	}
}

func TestNodeCertRoutes_Guards(t *testing.T) {
	rt, db := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodPost, "/api/v1/nodes/n1/reenroll-token"},
		{http.MethodPost, "/api/v1/nodes/n1/revoke-cert"},
	})

	bootstrapTestAdmin(t, db)
	seedCertNode(t, db, "n1", time.Hour, "")
	seedCertNode(t, db, "n2", time.Hour, "")
	root := storeUserWithAbilitiesForTest(t, db, "ops@example.com", []string{AbilityRoot})
	attachTestPolicy(t, db, "deny-n1", "Deny", AbilityRoot, "node:n1", store.PrincipalTypeUser, root.ID)
	cookie := sessionCookieForTest(t, rt, root.ID)

	for _, tt := range []struct {
		target string
		want   int
	}{
		{"/api/v1/nodes/n1/revoke-cert", http.StatusForbidden},
		{"/api/v1/nodes/n1/reenroll-token", http.StatusForbidden},
		{"/api/v1/nodes/n2/reenroll-token", http.StatusCreated},
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, tt.target, ""))
		if rec.Code != tt.want {
			t.Errorf("POST %s status = %d, want %d (a node-scoped Deny must apply)", tt.target, rec.Code, tt.want)
		}
	}
}
