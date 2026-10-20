package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/store"
)

type rotatingTrustControl struct {
	fakeTrustControl
	db     *store.DB
	rotate bool
}

func (f *rotatingTrustControl) RequestRenew(id string) error {
	f.asked = append(f.asked, id)
	if f.err != nil || !f.rotate {
		return f.err
	}
	now := time.Now()
	return f.db.RotateNodeCert(context.Background(), id, "fp-"+id, store.NodeCert{
		Fingerprint: "fp2-" + id, Serial: "serial-2", NotAfter: now.Add(24 * time.Hour), KeyOrigin: store.CertKeyOriginAgent,
	}, time.Hour, now)
}

func runDrill(t *testing.T, rt *Router, cookie *http.Cookie, node string) (int, nodeCertDrillResult) {
	t.Helper()
	rec := trustReq(t, rt, cookie, http.MethodPost, "/api/v1/nodes/"+node+"/cert/drill")
	var out nodeCertDrillResult
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, out
}

func stepStatus(res nodeCertDrillResult, id string) string {
	for _, s := range res.Steps {
		if s.ID == id {
			return s.Status
		}
	}
	return ""
}

func TestNodeCertDrill(t *testing.T) {
	tests := []struct {
		name       string
		rotate     bool
		reqErr     error
		revoke     bool
		offline    bool
		wantOK     bool
		failedStep string
	}{
		{name: "rotates and stays online", rotate: true, wantOK: true},
		{name: "agent never answers", failedStep: drillStepIssued},
		{name: "not connected", reqErr: agent.ErrNodeNotConnected, failedStep: drillStepRequest},
		{name: "revoked is refused before asking", revoke: true, failedStep: drillStepPreflight},
		{name: "offline is refused before asking", offline: true, failedStep: drillStepPreflight},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			tc := &rotatingTrustControl{db: db, rotate: tt.rotate}
			tc.err = tt.reqErr
			WithNodeTrustControl(tc)(rt)
			rt.nodeCerts.drillTimeout = 300 * time.Millisecond
			rt.nodeCerts.drillPoll = 20 * time.Millisecond
			cookie := loginTestSession(t, rt, db)
			seedCertNode(t, db, "n1", time.Hour, "")
			ctx := context.Background()
			if tt.revoke {
				_ = db.RevokeNodeCert(ctx, "n1", time.Now())
			}
			if tt.offline {
				if err := db.UpdateNodeStatus(ctx, "n1", store.NodeStatusOffline); err != nil {
					t.Fatal(err)
				}
			}
			code, res := runDrill(t, rt, cookie, "n1")
			if code != http.StatusOK {
				t.Fatalf("status = %d", code)
			}
			if res.OK != tt.wantOK {
				t.Fatalf("ok = %v, want %v: %+v", res.OK, tt.wantOK, res.Steps)
			}
			if tt.failedStep != "" && stepStatus(res, tt.failedStep) != drillFail {
				t.Fatalf("step %s = %q, want fail: %+v", tt.failedStep, stepStatus(res, tt.failedStep), res.Steps)
			}
			if tt.wantOK && (res.After.Serial != "serial-2" || res.After.Generation <= res.Before.Generation) {
				t.Fatalf("after = %+v before = %+v, want a newer certificate", res.After, res.Before)
			}
			if tt.revoke || tt.offline {
				if len(tc.asked) != 0 {
					t.Fatalf("agent was asked %v despite a failed preflight", tc.asked)
				}
			}
			n, err := db.GetNode(ctx, "n1")
			if err != nil {
				t.Fatal(err)
			}
			if !tt.wantOK && !tt.revoke && n.CertRevokedAt != nil {
				t.Fatal("a failed drill must never revoke the certificate")
			}
		})
	}
}

func TestNodeCertDrill_LocalAndAuth(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.SetLocalNodeID("local")
	cookie := loginTestSession(t, rt, db)
	seedCertNode(t, db, "local", time.Hour, "")
	if code, _ := runDrill(t, rt, cookie, "local"); code != http.StatusBadRequest {
		t.Fatalf("local drill = %d, want 400", code)
	}
	if code, _ := runDrill(t, rt, cookie, "ghost"); code != http.StatusNotFound {
		t.Fatalf("unknown node = %d, want 404", code)
	}
	assertRoutesRequireAuth(t, rt, []routeCase{{http.MethodPost, "/api/v1/nodes/n1/cert/drill"}})
	reader := storeUserWithAbilitiesForTest(t, db, "viewer@example.com", []string{AbilityRead})
	if rec := trustReq(t, rt, sessionCookieForTest(t, rt, reader.ID), http.MethodPost, "/api/v1/nodes/local/cert/drill"); rec.Code != http.StatusForbidden {
		t.Fatalf("read-only drill = %d, want 403", rec.Code)
	}
}
