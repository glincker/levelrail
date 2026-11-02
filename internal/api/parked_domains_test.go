package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestParkedDomainsAPI_Lifecycle(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, svc := range []store.DesiredService{
		{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"a.example.com", "b.example.com"}},
		{Name: "api", Image: "img:v1", Port: 80},
	} {
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			t.Fatal(err)
		}
	}

	rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/park", `{"app":"web","domain":"B.example.com","note":"later"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("park = %d %s", rec.Code, rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodGet, "/api/v1/domains/parked", "")
	if !strings.Contains(rec.Body.String(), `"domain":"b.example.com"`) || !strings.Contains(rec.Body.String(), `"parked_from":"web"`) {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}

	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/move", `{"domain":"a.example.com","from":"web","to":"api"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "requires_confirmation") {
		t.Fatalf("unconfirmed move = %d %s, want 409 asking for confirmation", rec.Code, rec.Body.String())
	}
	if svc, _ := db.GetDesiredService(ctx, "web"); len(svc.Domains) != 1 {
		t.Fatalf("unconfirmed move changed web: %v", svc.Domains)
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/move", `{"domain":"a.example.com","from":"web","to":"api","confirm":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("move = %d %s", rec.Code, rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/move", `{"domain":"a.example.com","from":"web","to":"api","confirm":true}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("move from wrong owner = %d, want 409", rec.Code)
	}

	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/parked/b.example.com/attach", `{"app":"api"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("attach = %d %s", rec.Code, rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/park", `{"app":"api","domain":"a.example.com"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-park = %d %s", rec.Code, rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodDelete, "/api/v1/domains/parked/a.example.com", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodDelete, "/api/v1/domains/parked/a.example.com", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d, want 404", rec.Code)
	}
}
