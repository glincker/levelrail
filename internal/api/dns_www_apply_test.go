package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/libdns/libdns"
)

const wwwApplyPath = "/api/v1/apps/web/domains/far.example.com/dns/apply"

// failSecondAppend lets the first record through and rejects the next.
type failSecondAppend struct {
	fakeDNSManager
	appends int
}

func (m *failSecondAppend) AppendRecords(ctx context.Context, z string, recs []libdns.Record) ([]libdns.Record, error) {
	m.appends++
	if m.appends > 1 {
		return nil, errors.New("provider rejected")
	}
	return m.fakeDNSManager.AppendRecords(ctx, z, recs)
}

func wwwApplyRouter(t *testing.T, mgr dnsrecords.Manager) (*Router, *http.Cookie) {
	t.Helper()
	var calls atomic.Int32
	rt, cookie := cutoverRouter(t, false, &calls)
	rt.domainAuto.resolveTarget = func(context.Context, string) (*dnsTarget, error) {
		return &dnsTarget{Manager: mgr, Provider: "cloudflare", Zone: "far.example.com."}, nil
	}
	return rt, cookie
}

func wwwApplyCall(t *testing.T, rt *Router, cookie *http.Cookie, body string) (int, wwwApplyResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, wwwApplyPath, body))
	var out wwwApplyResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v (%s)", err, rec.Body.String())
		}
	}
	return rec.Code, out
}

func TestWWWApplyBothCreatesAndAudits(t *testing.T) {
	mgr := &fakeDNSManager{}
	rt, cookie := wwwApplyRouter(t, mgr)
	code, out := wwwApplyCall(t, rt, cookie, `{"mode":"both","canonical":"apex","confirm":true}`)
	if code != http.StatusOK || !out.Applied {
		t.Fatalf("status = %d applied = %v: %+v", code, out.Applied, out)
	}
	if len(mgr.recs) != 2 || mgr.recs[0].RR().Name != "@" || mgr.recs[1].RR().Type != "CNAME" {
		t.Fatalf("zone = %+v", mgr.recs)
	}
	if len(out.Records) != 2 || out.Records[0].Result != "created" || out.Records[1].Result != "created" || len(out.Undo) != 2 {
		t.Fatalf("records = %+v undo = %+v", out.Records, out.Undo)
	}
	if out.RedirectApplied || out.RedirectMessage == "" {
		t.Fatalf("www twin is not attached, redirect must be reported not applied: %+v", out)
	}
	db, ok := rt.auditLog.(*store.DB)
	if !ok {
		t.Skip("audit log is not the sqlite store")
	}
	entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: store.AuditActionDNSRecordCreated})
	if err != nil || len(entries) != 2 {
		t.Fatalf("audit entries = %d err = %v, want 2", len(entries), err)
	}
}

func TestWWWApplyConflictWithoutReplaceWritesNothing(t *testing.T) {
	mgr := &fakeDNSManager{recs: []libdns.Record{libdns.RR{Name: "www", Type: "A", Data: "198.51.100.9", TTL: time.Minute}}}
	rt, cookie := wwwApplyRouter(t, mgr)
	code, out := wwwApplyCall(t, rt, cookie, `{"mode":"www_only","confirm":true}`)
	if code != http.StatusOK || out.Applied {
		t.Fatalf("status = %d applied = %v", code, out.Applied)
	}
	if len(out.Records) != 1 || out.Records[0].Result != "conflict" || len(mgr.recs) != 1 || mgr.recs[0].RR().Data != "198.51.100.9" {
		t.Fatalf("records = %+v zone = %+v", out.Records, mgr.recs)
	}
}

func TestWWWApplyRequiresConfirm(t *testing.T) {
	mgr := &fakeDNSManager{}
	rt, cookie := wwwApplyRouter(t, mgr)
	if code, _ := wwwApplyCall(t, rt, cookie, `{"mode":"both"}`); code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	if len(mgr.recs) != 0 {
		t.Fatalf("nothing may be written without confirm: %+v", mgr.recs)
	}
}

func TestWWWApplyFailureOnSecondRecordReturnsPartial(t *testing.T) {
	mgr := &failSecondAppend{}
	rt, cookie := wwwApplyRouter(t, mgr)
	code, out := wwwApplyCall(t, rt, cookie, `{"mode":"both","confirm":true}`)
	if code != http.StatusOK || out.Applied {
		t.Fatalf("status = %d applied = %v", code, out.Applied)
	}
	if len(out.Records) != 2 || out.Records[0].Result != "created" || out.Records[1].Result != "failed" {
		t.Fatalf("records = %+v", out.Records)
	}
	if len(out.Undo) != 1 || out.Undo[0].Name != "@" || out.RedirectApplied {
		t.Fatalf("undo = %+v redirect = %v", out.Undo, out.RedirectApplied)
	}
}
