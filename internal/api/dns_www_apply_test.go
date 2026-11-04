package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

// failTypeAppend rejects appends of one record type and accepts the rest.
type failTypeAppend struct {
	fakeDNSManager
	typ string
}

func (m *failTypeAppend) AppendRecords(ctx context.Context, z string, recs []libdns.Record) ([]libdns.Record, error) {
	for _, r := range recs {
		if r.RR().Type == m.typ {
			return nil, errors.New("provider rejected")
		}
	}
	return m.fakeDNSManager.AppendRecords(ctx, z, recs)
}

func wwwHasA(recs []libdns.Record, name, data string) bool {
	for _, r := range recs {
		if rr := r.RR(); rr.Name == name && rr.Type == "A" && rr.Data == data {
			return true
		}
	}
	return false
}

func TestWWWApplyRejectsApexCNAMEOnProviderWithoutFlattening(t *testing.T) {
	mgr := &fakeDNSManager{recs: []libdns.Record{libdns.RR{Name: "@", Type: "A", Data: "198.51.100.9", TTL: time.Minute}}}
	var calls atomic.Int32
	rt, cookie := cutoverRouter(t, false, &calls)
	rt.publicHost = "edge.example.net"
	rt.domainAuto.resolveTarget = func(context.Context, string) (*dnsTarget, error) {
		return &dnsTarget{Manager: mgr, Provider: "route53", Zone: "far.example.com."}, nil
	}
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/domains/far.example.com/dns/plan?mode=apex_only", nil); code != http.StatusConflict {
		t.Fatalf("plan status = %d, want 409", code)
	}
	if code, _ := wwwApplyCall(t, rt, cookie, `{"mode":"apex_only","replace":true,"confirm":true}`); code != http.StatusConflict {
		t.Fatalf("apply status = %d, want 409", code)
	}
	if len(mgr.recs) != 1 || mgr.recs[0].RR().Data != "198.51.100.9" {
		t.Fatalf("the apex A record must survive: %+v", mgr.recs)
	}
}

func TestWWWApplyRestoresReplacedRecordWhenAppendFails(t *testing.T) {
	mgr := &failTypeAppend{typ: "CNAME", fakeDNSManager: fakeDNSManager{recs: []libdns.Record{
		libdns.RR{Name: "@", Type: "A", Data: "198.51.100.9", TTL: time.Minute}}}}
	var calls atomic.Int32
	rt, cookie := cutoverRouter(t, false, &calls)
	rt.publicHost = "edge.example.net"
	rt.domainAuto.resolveTarget = func(context.Context, string) (*dnsTarget, error) {
		return &dnsTarget{Manager: mgr, Provider: "cloudflare", Zone: "far.example.com."}, nil
	}
	code, out := wwwApplyCall(t, rt, cookie, `{"mode":"apex_only","replace":true,"confirm":true}`)
	if code != http.StatusOK || out.Applied || out.Records[0].Result != "failed" {
		t.Fatalf("status = %d out = %+v", code, out)
	}
	if !wwwHasA(mgr.recs, "@", "198.51.100.9") {
		t.Fatalf("the deleted A record was not restored: %+v", mgr.recs)
	}
	if !strings.Contains(out.Records[0].Message, "restored") || len(out.Undo) != 0 {
		t.Fatalf("message = %q undo = %+v", out.Records[0].Message, out.Undo)
	}
}

func TestWWWApplyPersistsRunAndUndoRestores(t *testing.T) {
	mgr := &failSecondAppend{fakeDNSManager: fakeDNSManager{recs: []libdns.Record{
		libdns.RR{Name: "www", Type: "A", Data: "198.51.100.9", TTL: time.Minute}}}}
	rt, cookie := wwwApplyRouter(t, mgr)
	code, out := wwwApplyCall(t, rt, cookie, `{"mode":"both","replace":true,"confirm":true}`)
	if code != http.StatusOK || out.Applied || out.RunID == "" {
		t.Fatalf("status = %d applied = %v run = %q", code, out.Applied, out.RunID)
	}
	if wwwHasA(mgr.recs, "www", "198.51.100.9") {
		t.Fatalf("restore was meant to fail in this setup: %+v", mgr.recs)
	}
	mgr.appends = 0
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/domains/automation/runs/"+out.RunID+"/undo", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("undo status = %d: %s", rec.Code, rec.Body.String())
	}
	if !wwwHasA(mgr.recs, "www", "198.51.100.9") || wwwHasA(mgr.recs, "@", "203.0.113.7") {
		t.Fatalf("undo should restore www and drop the apex: %+v", mgr.recs)
	}
}

func TestWWWApplyTracksApexAndWWWSeparately(t *testing.T) {
	mgr := &fakeDNSManager{}
	var calls atomic.Int32
	rt, cookie := cutoverRouter(t, false, &calls)
	rt.publicHost = "edge.example.net"
	rt.domainAuto.resolveTarget = func(context.Context, string) (*dnsTarget, error) {
		return &dnsTarget{Manager: mgr, Provider: "cloudflare", Zone: "far.example.com."}, nil
	}
	if code, out := wwwApplyCall(t, rt, cookie, `{"mode":"both","confirm":true}`); code != http.StatusOK || !out.Applied {
		t.Fatalf("status = %d out = %+v", code, out)
	}
	ms := rt.managedDNS()
	apex, _ := ms.ListManagedDNSRecords(context.Background(), "far.example.com")
	www, _ := ms.ListManagedDNSRecords(context.Background(), "www.far.example.com")
	if len(apex) != 1 || apex[0].Name != "@" || len(www) != 1 || www[0].Name != "www" {
		t.Fatalf("apex rows = %+v www rows = %+v", apex, www)
	}
}

func TestWWWApplyForeignOwnedWWWIsConflictEvenWithReplace(t *testing.T) {
	mgr := &fakeDNSManager{recs: []libdns.Record{libdns.RR{Name: "www", Type: "A", Data: "198.51.100.9", TTL: time.Minute}}}
	rt, cookie := wwwApplyRouter(t, mgr)
	db := rt.auditLog.(*store.DB)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "other", Image: "levelrail/other:1", Port: 3000, Domains: []string{"www.far.example.com"}}); err != nil {
		t.Fatal(err)
	}
	code, out := wwwApplyCall(t, rt, cookie, `{"mode":"both","replace":true,"confirm":true}`)
	if code != http.StatusOK || out.Applied {
		t.Fatalf("status = %d applied = %v", code, out.Applied)
	}
	if out.Records[1].Result != "conflict" || !strings.Contains(out.Records[1].Message, "other") {
		t.Fatalf("www record = %+v", out.Records[1])
	}
	if !wwwHasA(mgr.recs, "www", "198.51.100.9") {
		t.Fatalf("the other app's www record was overwritten: %+v", mgr.recs)
	}
	if out.RedirectApplied || !strings.Contains(out.RedirectMessage, "conflict") {
		t.Fatalf("redirect must be skipped on a conflict: %+v", out)
	}
}

func TestWWWApplyReplaceOverwritesConflict(t *testing.T) {
	mgr := &fakeDNSManager{recs: []libdns.Record{libdns.RR{Name: "www", Type: "A", Data: "198.51.100.9", TTL: time.Minute}}}
	rt, cookie := wwwApplyRouter(t, mgr)
	code, out := wwwApplyCall(t, rt, cookie, `{"mode":"www_only","replace":true,"confirm":true}`)
	if code != http.StatusOK || !out.Applied || out.Records[0].Result != "updated" || out.RunID == "" {
		t.Fatalf("status = %d out = %+v", code, out)
	}
	if wwwHasA(mgr.recs, "www", "198.51.100.9") || len(mgr.recs) != 1 {
		t.Fatalf("zone = %+v", mgr.recs)
	}
}

func TestWWWApplyRedirectAppliedWhenTwinAttached(t *testing.T) {
	mgr := &fakeDNSManager{}
	rt, cookie := wwwApplyRouter(t, mgr)
	db := rt.auditLog.(*store.DB)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000, Domains: []string{"far.example.com", "www.far.example.com"}}); err != nil {
		t.Fatal(err)
	}
	code, out := wwwApplyCall(t, rt, cookie, `{"mode":"both","canonical":"apex","confirm":true}`)
	if code != http.StatusOK || !out.Applied || !out.RedirectApplied {
		t.Fatalf("status = %d out = %+v", code, out)
	}
}

func TestWWWApplyNonRootForbidden(t *testing.T) {
	mgr := &fakeDNSManager{}
	rt, _ := wwwApplyRouter(t, mgr)
	db := rt.auditLog.(*store.DB)
	u := storeUserWithAbilitiesForTest(t, db, "deployer@example.com", []string{AbilityRead, AbilityDeploy})
	cookie := sessionCookieForTest(t, rt, u.ID)
	if code, _ := wwwApplyCall(t, rt, cookie, `{"mode":"both","confirm":true}`); code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", code)
	}
	if len(mgr.recs) != 0 {
		t.Fatalf("nothing may be written: %+v", mgr.recs)
	}
}
