package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleListDeployTargets_Groups(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/deploy-targets", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got []deployTargetGroup
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 || got[0].Kind != deployTargetKindLocal || got[1].Kind != deployTargetKindUnmanaged {
		t.Fatalf("groups = %+v", got)
	}
	if got[1].Nodes[0].Name != "alpha" {
		t.Errorf("unmanaged node = %+v", got[1].Nodes[0])
	}
}

func TestHandleListDeployTargets_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/deploy-targets", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
