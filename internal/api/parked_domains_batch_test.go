package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func decodeBatch(t *testing.T, body string) domainBatchResponse {
	t.Helper()
	var out domainBatchResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return out
}

func TestParkedDomainsBatch_PartialFailureAndAudit(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, svc := range []store.DesiredService{
		{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"a.example.com", "b.example.com"}},
		{Name: "api", Image: "img:v1", Port: 80, Domains: []string{"c.example.com"}},
	} {
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			t.Fatal(err)
		}
	}
	body := `{"action":"park","items":[
		{"domain":"A.example.com","app":"web"},
		{"domain":"missing.example.com","app":"web"},
		{"domain":"a.example.com","app":"web"},
		{"domain":"c.example.com","app":"ghost"},
		{"domain":"b.example.com","app":"web","note":"later"}]}`
	rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/batch", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("batch = %d %s", rec.Code, rec.Body.String())
	}
	got := decodeBatch(t, rec.Body.String())
	if len(got.Results) != 5 || got.Summary.Succeeded != 2 || got.Summary.Failed != 3 || got.BatchID == "" {
		t.Fatalf("summary = %+v", got.Summary)
	}
	if !got.Results[0].OK || got.Results[1].OK || got.Results[1].Status != http.StatusBadRequest {
		t.Fatalf("results = %+v", got.Results)
	}
	if got.Results[2].Code != errCodeDuplicateDomain || got.Results[2].Status != http.StatusBadRequest {
		t.Fatalf("duplicate = %+v", got.Results[2])
	}
	if got.Results[3].Status != http.StatusNotFound || !got.Results[4].OK {
		t.Fatalf("results = %+v", got.Results)
	}
	if svc, _ := db.GetDesiredService(ctx, "web"); len(svc.Domains) != 0 {
		t.Fatalf("web domains = %v, want both parked", svc.Domains)
	}
	entries, err := db.ListAuditEntries(ctx, 50, nil, store.AuditEntryFilter{Action: auditActionDomainParked})
	if err != nil || len(entries) != 2 {
		t.Fatalf("audit entries = %d, %v", len(entries), err)
	}
	for _, e := range entries {
		if !strings.Contains(e.Detail, got.BatchID) {
			t.Fatalf("audit detail %q lacks batch id %s", e.Detail, got.BatchID)
		}
	}
}

func TestParkedDomainsBatch_MoveCollisionNeedsConfirm(t *testing.T) {
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
	items := `[{"domain":"a.example.com","from":"web","to":"api"},{"domain":"nope.example.com","from":"web","to":"api"}]`
	rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/batch", `{"action":"move","items":`+items+`}`)
	got := decodeBatch(t, rec.Body.String())
	if rec.Code != http.StatusOK || !got.Results[0].RequiresConfirmation || got.Results[0].OK || got.Summary.RequiresConfirmation != 1 || got.Summary.Failed != 1 {
		t.Fatalf("unconfirmed = %d %s", rec.Code, rec.Body.String())
	}
	if svc, _ := db.GetDesiredService(ctx, "web"); len(svc.Domains) != 2 {
		t.Fatalf("unconfirmed batch changed web: %v", svc.Domains)
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/batch", `{"action":"move","confirm":true,"items":`+items+`}`)
	got = decodeBatch(t, rec.Body.String())
	if !got.Results[0].OK || got.Results[1].Status != http.StatusConflict {
		t.Fatalf("confirmed = %s", rec.Body.String())
	}
	if svc, _ := db.GetDesiredService(ctx, "api"); len(svc.Domains) != 1 {
		t.Fatalf("api domains = %v", svc.Domains)
	}
}

func TestParkedDomainsBatch_AttachAndDeleteParked(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"a.example.com", "b.example.com"}}); err != nil {
		t.Fatal(err)
	}
	rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/batch", `{"action":"park","items":[{"domain":"a.example.com","app":"web"},{"domain":"b.example.com","app":"web"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("park = %s", rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/batch", `{"action":"attach","items":[{"domain":"a.example.com","app":"web"}]}`)
	if got := decodeBatch(t, rec.Body.String()); !got.Results[0].OK {
		t.Fatalf("attach = %s", rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/batch", `{"action":"delete_parked","items":[{"domain":"b.example.com"},{"domain":"b.example.com"},{"domain":"zzz.example.com"}]}`)
	got := decodeBatch(t, rec.Body.String())
	if !got.Results[0].OK || got.Results[0].Status != http.StatusNoContent || got.Results[1].Code != errCodeDuplicateDomain || got.Results[2].Status != http.StatusNotFound {
		t.Fatalf("delete = %s", rec.Body.String())
	}
}

func TestParkedDomainsBatch_RejectsBadRequests(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	var many []string
	for i := range 51 {
		many = append(many, fmt.Sprintf(`{"domain":"d%d.example.com","app":"web"}`, i))
	}
	for name, tc := range map[string]struct{ body, want string }{
		"over max":       {`{"action":"park","items":[` + strings.Join(many, ",") + `]}`, errCodeBatchTooLarge},
		"unknown action": {`{"action":"nuke","items":[{"domain":"a.example.com"}]}`, "action must be"},
		"empty":          {`{"action":"park","items":[]}`, "must not be empty"},
	} {
		rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/batch", tc.body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s = %d %s", name, rec.Code, rec.Body.String())
		}
	}
}

func TestParkedDomainsBatch_DeniedItemDoesNotStopOthers(t *testing.T) {
	rt, cookie := staticDenyUser(t, "*", "app:blog")
	ctx := context.Background()
	db := rt.apps.(*store.DB)
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"w.example.com"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "blog", Image: "img:v1", Port: 80, Domains: []string{"x.example.com"}}); err != nil {
		t.Fatal(err)
	}
	rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/batch", `{"action":"park","items":[{"domain":"x.example.com","app":"blog"},{"domain":"w.example.com","app":"web"}]}`)
	got := decodeBatch(t, rec.Body.String())
	if rec.Code != http.StatusOK || got.Results[0].OK || got.Results[0].Status != http.StatusNotFound || !got.Results[1].OK {
		t.Fatalf("batch = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "denied") || strings.Contains(rec.Body.String(), "policy") {
		t.Fatalf("denial leaks policy detail: %s", rec.Body.String())
	}
	if svc, _ := db.GetDesiredService(ctx, "blog"); len(svc.Domains) != 1 {
		t.Fatalf("denied item changed blog: %v", svc.Domains)
	}
}
