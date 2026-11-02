package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/store"
)

type resyncFake struct {
	mu      sync.Mutex
	records map[string]bool // record id -> proxied
	content map[string]string
	patched []string
	failID  string
}

func newResyncFixture(t *testing.T, f *resyncFake, proxied bool) (*Router, *http.Cookie) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": v}) }
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == "/zones":
			ok([]map[string]string{{"id": "z1", "name": "example.com"}})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/dns_records"):
			name := r.URL.Query().Get("name")
			id := "rec-" + name
			ok([]map[string]any{{"id": id, "content": f.content[id], "proxied": f.records[id]}})
		case r.Method == http.MethodPatch:
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			if id == f.failID {
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "errors": []map[string]string{{"message": "denied"}}})
				return
			}
			var b struct{ Proxied bool }
			_ = json.NewDecoder(r.Body).Decode(&b)
			f.records[id] = b.Proxied
			f.patched = append(f.patched, id)
			ok(map[string]string{"id": id})
		}
	}))
	t.Cleanup(srv.Close)
	rt, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "img:v1", Port: 8080, Domains: []string{"a.example.com", "b.example.com", "c.example.com"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppDNSProxied(ctx, "web", &proxied, "t"); err != nil {
		t.Fatal(err)
	}
	// a and b are tracked for web; c is a record someone else made.
	for _, d := range []string{"a.example.com", "b.example.com"} {
		if err := db.SaveManagedDNSRecord(ctx, store.ManagedDNSRecord{Domain: d, Provider: "cloudflare", Zone: "example.com.", Name: strings.TrimSuffix(d, ".example.com"), RecordType: "A", Value: "192.0.2.1", AppName: "web"}); err != nil {
			t.Fatal(err)
		}
	}
	api := &dnsrecords.CloudflareAPI{Token: cdnTestToken, BaseURL: srv.URL}
	rt.domainAuto.resolveTarget = func(context.Context, string) (*dnsTarget, error) {
		return &dnsTarget{Provider: "cloudflare", Zone: "example.com.", Proxy: api}, nil
	}
	return rt, loginTestSession(t, rt, db)
}

func postResync(t *testing.T, rt *Router, cookie *http.Cookie, body string) appCDNResyncResult {
	t.Helper()
	rec := doPolicy(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/cdn/resync", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), cdnTestToken) {
		t.Fatal("token leaked")
	}
	var res appCDNResyncResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func TestResyncAppCDN_UpdatesOnlyOwnedRecords(t *testing.T) {
	f := &resyncFake{
		records: map[string]bool{"rec-a.example.com": false, "rec-b.example.com": true, "rec-c.example.com": false},
		content: map[string]string{"rec-a.example.com": "192.0.2.1", "rec-b.example.com": "192.0.2.1", "rec-c.example.com": "192.0.2.1"},
	}
	rt, cookie := newResyncFixture(t, f, true)
	res := postResync(t, rt, cookie, `{}`)
	if res.Updated != 1 || res.Failed != 0 || len(res.Records) != 2 {
		t.Fatalf("result = %+v", res)
	}
	if len(f.patched) != 1 || f.patched[0] != "rec-a.example.com" || f.records["rec-c.example.com"] {
		t.Fatalf("patched %v records %v", f.patched, f.records)
	}
	if res.Records[1].Result != resyncSame {
		t.Fatalf("already proxied record = %+v", res.Records[1])
	}
}

func TestResyncAppCDN_DryRunChangesNothing(t *testing.T) {
	f := &resyncFake{
		records: map[string]bool{"rec-a.example.com": false, "rec-b.example.com": true},
		content: map[string]string{"rec-a.example.com": "192.0.2.1", "rec-b.example.com": "192.0.2.1"},
	}
	rt, cookie := newResyncFixture(t, f, true)
	res := postResync(t, rt, cookie, `{"dry_run":true}`)
	if !res.DryRun || len(f.patched) != 0 || res.Records[0].Result != resyncWouldSet {
		t.Fatalf("dry run = %+v patched %v", res, f.patched)
	}
}

func TestResyncAppCDN_ReportsFailuresPerRecordAndLeavesChangedOnesAlone(t *testing.T) {
	f := &resyncFake{
		records: map[string]bool{"rec-a.example.com": false, "rec-b.example.com": false},
		content: map[string]string{"rec-a.example.com": "192.0.2.1", "rec-b.example.com": "203.0.113.9"},
		failID:  "rec-a.example.com",
	}
	rt, cookie := newResyncFixture(t, f, true)
	res := postResync(t, rt, cookie, `{}`)
	if res.Failed != 1 || res.Updated != 0 || res.Records[0].Result != resyncError || res.Records[1].Result != resyncSame {
		t.Fatalf("result = %+v", res)
	}
	if len(f.patched) != 0 || strings.Contains(res.Records[0].Message, "denied") {
		t.Fatalf("patched %v message %q", f.patched, res.Records[0].Message)
	}
}

func TestResyncAppCDN_TurningOffUnproxies(t *testing.T) {
	f := &resyncFake{
		records: map[string]bool{"rec-a.example.com": true, "rec-b.example.com": true},
		content: map[string]string{"rec-a.example.com": "192.0.2.1", "rec-b.example.com": "192.0.2.1"},
	}
	rt, cookie := newResyncFixture(t, f, false)
	res := postResync(t, rt, cookie, `{}`)
	if res.Updated != 2 || f.records["rec-a.example.com"] || f.records["rec-b.example.com"] {
		t.Fatalf("result = %+v records %v", res, f.records)
	}
}
