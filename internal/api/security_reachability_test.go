package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/exposure"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestSecurityReachabilityReportsExposedPort(t *testing.T) {
	ipt := &fakeIptables{rules: []string{"-j RETURN"}}
	rt, cookie := exposureRouter(t, ipt, nil)
	rt.sec.posture.mu.Lock()
	rt.sec.posture.exposureAt = time.Now()
	rt.sec.posture.mu.Unlock()

	code, body := exposureCall(t, rt, cookie, http.MethodGet, "/api/v1/security/reachability?refresh=true", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d: %s", code, body)
	}
	var rep exposure.ReachReport
	if err := json.Unmarshal([]byte(body), &rep); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rep.Findings == nil || rep.Subjects == nil {
		t.Fatalf("lists must be non-null: %s", body)
	}
	rt.sec.posture.mu.Lock()
	cleared := rt.sec.posture.exposureAt.IsZero()
	rt.sec.posture.mu.Unlock()
	if !cleared {
		t.Fatal("refresh=true did not drop the cached exposure audit")
	}
}

func TestSecurityReachabilityRequiresRoot(t *testing.T) {
	f := newTeamFixture(t)
	const path = "/api/v1/security/reachability"
	if got := matrixDo(f.rt, http.MethodGet, path, func(r *http.Request) { r.AddCookie(f.cookies[store.TeamRoleViewer]) }); got != http.StatusForbidden {
		t.Errorf("viewer = %d, want 403", got)
	}
	if got := matrixDo(f.rt, http.MethodGet, path, nil); got != http.StatusUnauthorized {
		t.Errorf("anonymous = %d, want 401", got)
	}
}
