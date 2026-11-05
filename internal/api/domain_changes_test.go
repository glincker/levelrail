package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/libdns/libdns"
)

const changesBase = "/api/v1/domains/far.example.com/changes"

func changesCall(t *testing.T, rt *Router, cookie *http.Cookie, method, path string, out any) int {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, ""))
	if out != nil && rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("decode: %v (%s)", err, rec.Body.String())
		}
	}
	return rec.Code
}

func appliedChange(t *testing.T, mgr *fakeDNSManager) (*Router, *http.Cookie, string) {
	t.Helper()
	rt, cookie := wwwApplyRouter(t, mgr)
	code, out := wwwApplyCall(t, rt, cookie, `{"mode":"both","confirm":true}`)
	if code != http.StatusOK || !out.Applied || out.RunID == "" {
		t.Fatalf("apply status = %d out = %+v", code, out)
	}
	return rt, cookie, out.RunID
}

func TestChangesListAndRollbackRemovesOnlyProductRecords(t *testing.T) {
	foreign := libdns.RR{Name: "api", Type: "A", Data: "192.0.2.50", TTL: time.Minute}
	mgr := &fakeDNSManager{recs: []libdns.Record{foreign}}
	rt, cookie, id := appliedChange(t, mgr)

	var list []domainChange
	if code := changesCall(t, rt, cookie, http.MethodGet, changesBase, &list); code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list status = %d list = %+v", code, list)
	}
	if c := list[0]; c.RunID != id || !c.CanRollback || c.AlreadyUndone || len(c.Summary) != 2 || !c.RollbackUntil.After(c.CreatedAt) {
		t.Fatalf("change = %+v", c)
	}
	var res rollbackResponse
	if code := changesCall(t, rt, cookie, http.MethodPost, changesBase+"/"+id+"/rollback", &res); code != http.StatusOK {
		t.Fatalf("rollback status = %d", code)
	}
	for _, o := range res.Outcomes {
		if o.Status != rollbackReverted {
			t.Fatalf("outcome = %+v", o)
		}
	}
	if len(mgr.recs) != 1 || mgr.recs[0].RR().Name != "api" {
		t.Fatalf("only the foreign record should remain: %+v", mgr.recs)
	}
	changesCall(t, rt, cookie, http.MethodGet, changesBase, &list)
	if list[0].CanRollback || !list[0].AlreadyUndone {
		t.Fatalf("after rollback = %+v", list[0])
	}
	if code := changesCall(t, rt, cookie, http.MethodPost, changesBase+"/"+id+"/rollback", nil); code != http.StatusConflict {
		t.Fatalf("second rollback status = %d, want 409", code)
	}
}

func TestRollbackWindowClosedIs409(t *testing.T) {
	mgr := &fakeDNSManager{}
	rt, cookie, id := appliedChange(t, mgr)
	db := rt.auditLog.(*store.DB)
	old := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(context.Background(), `UPDATE domain_automation_runs SET created_at = ? WHERE id = ?`, old, id); err != nil {
		t.Fatal(err)
	}
	var list []domainChange
	changesCall(t, rt, cookie, http.MethodGet, changesBase, &list)
	if len(list) != 1 || list[0].CanRollback {
		t.Fatalf("list = %+v", list)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, changesBase+"/"+id+"/rollback", ""))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "window closed") {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if len(mgr.recs) != 2 {
		t.Fatalf("records must be untouched: %+v", mgr.recs)
	}
}

func TestRollbackSkipsRecordChangedSince(t *testing.T) {
	mgr := &fakeDNSManager{}
	rt, cookie, id := appliedChange(t, mgr)
	for i, rec := range mgr.recs {
		if rr := rec.RR(); rr.Name == "@" {
			rr.Data = "198.51.100.77"
			mgr.recs[i] = rr
		}
	}
	var res rollbackResponse
	if code := changesCall(t, rt, cookie, http.MethodPost, changesBase+"/"+id+"/rollback", &res); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	skipped, reverted := 0, 0
	for _, o := range res.Outcomes {
		switch o.Status {
		case rollbackSkipped:
			skipped++
			if !strings.Contains(o.Message, "198.51.100.77") {
				t.Fatalf("message = %q", o.Message)
			}
		case rollbackReverted:
			reverted++
		}
	}
	if skipped != 1 || reverted != 1 || len(mgr.recs) != 1 || mgr.recs[0].RR().Data != "198.51.100.77" {
		t.Fatalf("outcomes = %+v recs = %+v", res.Outcomes, mgr.recs)
	}
}

func TestRollbackWrongDomainIs404(t *testing.T) {
	mgr := &fakeDNSManager{}
	rt, cookie, id := appliedChange(t, mgr)
	if code := changesCall(t, rt, cookie, http.MethodPost, "/api/v1/domains/other.example.com/changes/"+id+"/rollback", nil); code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	if len(mgr.recs) != 2 {
		t.Fatalf("records must be untouched: %+v", mgr.recs)
	}
}

func TestRollbackNeedsRootAuthz(t *testing.T) {
	mgr := &fakeDNSManager{}
	rt, cookie, id := appliedChange(t, mgr)
	_ = cookie
	db := rt.auditLog.(*store.DB)
	reader := storeUserWithAbilitiesForTest(t, db, "reader-changes@example.com", []string{AbilityRead})
	rc := sessionCookieForTest(t, rt, reader.ID)
	if code := changesCall(t, rt, rc, http.MethodPost, changesBase+"/"+id+"/rollback", nil); code != http.StatusForbidden {
		t.Fatalf("rollback status = %d, want 403", code)
	}
	if code := changesCall(t, rt, rc, http.MethodGet, changesBase, nil); code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", code)
	}
}
