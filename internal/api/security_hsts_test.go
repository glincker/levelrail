package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func hstsCall(t *testing.T, rt *Router, cookie *http.Cookie, method, body string) (int, hstsProbeResult) {
	t.Helper()
	rec := serve(rt, authedRequest(t, cookie, method, "/api/v1/security/hsts", body))
	var out hstsProbeResult
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestHSTS_OptInOnlyAfterTrustedProbe(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.UpdateIngressSettings(ctx, store.IngressSettings{PrimaryDomain: "panel.example.com"}); err != nil {
		t.Fatal(err)
	}

	code, res := hstsCall(t, rt, cookie, http.MethodGet, "")
	if code != http.StatusOK || res.Enabled {
		t.Fatalf("default = %d enabled=%v, want 200 and off", code, res.Enabled)
	}

	rt.hstsHostProbe = func(_ context.Context, host string, _ int) hstsHostResult {
		return hstsHostResult{Host: host, Error: "the certificate is self-signed or from the internal issuer"}
	}
	code, res = hstsCall(t, rt, cookie, http.MethodPut, `{"enabled":true}`)
	if code != http.StatusConflict || res.Ready {
		t.Fatalf("self-signed enable = %d ready=%v, want 409", code, res.Ready)
	}
	if rt.hstsEnabledFromDB(ctx) {
		t.Fatal("HSTS turned on despite a failed probe")
	}
	if s, _ := db.GetIngressSettings(ctx); s.HSTSEnabled {
		t.Fatal("HSTS persisted despite a failed probe")
	}

	rt.hstsHostProbe = func(_ context.Context, host string, _ int) hstsHostResult {
		return hstsHostResult{Host: host, Verified: true, Issuer: "Let's Encrypt"}
	}
	code, res = hstsCall(t, rt, cookie, http.MethodPut, `{"enabled":true}`)
	if code != http.StatusOK || !res.Enabled || !res.Ready {
		t.Fatalf("trusted enable = %d enabled=%v ready=%v", code, res.Enabled, res.Ready)
	}

	rt.hstsHostProbe = func(_ context.Context, host string, _ int) hstsHostResult {
		return hstsHostResult{Host: host, Error: "down"}
	}
	code, res = hstsCall(t, rt, cookie, http.MethodPut, `{"enabled":false}`)
	if code != http.StatusOK || res.Enabled {
		t.Fatalf("disable with failing probe = %d enabled=%v, want 200 and off", code, res.Enabled)
	}
}

func TestHSTS_NoDomainIsNeverReady(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.hstsHostProbe = func(_ context.Context, host string, _ int) hstsHostResult {
		return hstsHostResult{Host: host, Verified: true}
	}
	if code, res := hstsCall(t, rt, cookie, http.MethodPut, `{"enabled":true}`); code != http.StatusConflict || res.Ready {
		t.Fatalf("no-domain enable = %d ready=%v, want 409", code, res.Ready)
	}
}

func TestHSTS_RequiresRoot(t *testing.T) {
	f := newTeamFixture(t)
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		if got := matrixDo(f.rt, method, "/api/v1/security/hsts", func(r *http.Request) { r.AddCookie(f.cookies[store.TeamRoleViewer]) }); got != http.StatusForbidden {
			t.Errorf("viewer %s = %d, want 403", method, got)
		}
		if got := matrixDo(f.rt, method, "/api/v1/security/hsts", nil); got != http.StatusUnauthorized {
			t.Errorf("anonymous %s = %d, want 401", method, got)
		}
	}
}
