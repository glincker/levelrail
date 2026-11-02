package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

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

func TestParkedDomainsAPI_AttachSettings(t *testing.T) {
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
	for _, d := range []string{"a.example.com", "b.example.com"} {
		if err := db.SetDomainWAF(ctx, d, true, store.DomainWAFModeBlock, 5, 5); err != nil {
			t.Fatal(err)
		}
		if rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/park", `{"app":"web","domain":"`+d+`"}`); rec.Code != http.StatusOK {
			t.Fatalf("park = %d %s", rec.Code, rec.Body.String())
		}
	}
	rec := sdReq(t, rt, cookie, http.MethodGet, "/api/v1/domains/parked", "")
	if !strings.Contains(rec.Body.String(), `"has_settings":true`) {
		t.Fatalf("list = %s, want has_settings", rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/parked/a.example.com/attach", `{"app":"api"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"restored_settings":["domain_waf"]`) {
		t.Fatalf("attach = %d %s, want waf restored", rec.Code, rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/parked/b.example.com/attach", `{"app":"api","discard_settings":true}`)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "restored_settings") {
		t.Fatalf("discard attach = %d %s", rec.Code, rec.Body.String())
	}
	if _, found, _ := db.GetDomainWAF(ctx, "b.example.com"); found {
		t.Fatal("discarded settings came back")
	}
}

func TestParkedDomainsAPI_LiveState(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	rt.publicHost = "203.0.113.7"
	rt.detectPublicIPs = func(context.Context) []string { return nil }
	rt.publicLookup = func(context.Context, string) []resolverResult { return nil }
	rt.lookupHost = func(_ context.Context, host string) ([]string, error) {
		switch host {
		case "ok.example.com":
			return []string{"203.0.113.7"}, nil
		case "far.example.com":
			return []string{"198.51.100.4"}, nil
		}
		return []string{"203.0.113.7"}, nil
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"ok.example.com", "far.example.com", "old.example.com"}}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	seedCert(t, db, "ok.example.com", now.Add(-24*time.Hour), now.Add(10*24*time.Hour))
	seedCert(t, db, "old.example.com", now.Add(-90*24*time.Hour), now.Add(-2*24*time.Hour))
	for _, d := range []string{"ok.example.com", "far.example.com", "old.example.com"} {
		if rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/park", `{"app":"web","domain":"`+d+`"}`); rec.Code != http.StatusOK {
			t.Fatalf("park %s = %d %s", d, rec.Code, rec.Body.String())
		}
	}

	rec := sdReq(t, rt, cookie, http.MethodGet, "/api/v1/domains/parked", "")
	var got []parkedDomainResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	by := map[string]parkedDomainResource{}
	for _, p := range got {
		by[p.Domain] = p
	}
	if p := by["ok.example.com"]; p.DNS == nil || p.DNS.Status != parkedDNSPointing || p.TLS == nil || p.TLS.State != parkedTLSIssued || p.TLS.DaysLeft == nil || *p.TLS.DaysLeft < 9 {
		t.Fatalf("ok row = %+v", p)
	}
	if p := by["far.example.com"]; p.DNS.Status != parkedDNSNotPointing || p.TLS.State != parkedTLSNone || p.TLS.DaysLeft != nil {
		t.Fatalf("far row (no cert) = %+v %+v", p.DNS, p.TLS)
	}
	if p := by["old.example.com"]; p.TLS.State != parkedTLSIssued || p.TLS.DaysLeft == nil || *p.TLS.DaysLeft >= 0 {
		t.Fatalf("old row (expired) = %+v", p.TLS)
	}
}

func TestParkedDNSFromCheck(t *testing.T) {
	for in, want := range map[string]string{
		domainCheckStatusConnected: parkedDNSPointing, domainCheckStatusPropagating: parkedDNSPointing,
		domainCheckStatusNotResolving: parkedDNSNotPointing, domainCheckStatusResolvesElsewhere: parkedDNSNotPointing,
		domainCheckStatusUnconfigured: parkedDNSUnknown, "": parkedDNSUnknown,
	} {
		if got := parkedDNSFromCheck(in); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}

func TestParkedDomainsAPI_Environment(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, svc := range []store.DesiredService{
		{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"www.example.com"}},
		{Name: "api", Image: "img:v1", Port: 80},
	} {
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []store.Environment{{ID: "env_stg", Name: "stg"}, {ID: "env_prod", Name: "prod"}} {
		e.ProjectID, e.Scope = "proj_global", store.EnvironmentScopeGlobal
		if err := db.SaveEnvironment(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := db.EditServiceEnvironmentDomains(ctx, "web", "env_stg", func([]string) ([]string, error) {
		return []string{"stg.example.com", "x.example.com"}, nil
	}); err != nil {
		t.Fatal(err)
	}

	rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/park", `{"app":"web","domain":"stg.example.com","environment":"stg"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"environment":"stg"`) {
		t.Fatalf("park env = %d %s", rec.Code, rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodGet, "/api/v1/domains/parked", "")
	if !strings.Contains(rec.Body.String(), `"parked_from_environment":"stg"`) {
		t.Fatalf("list = %s", rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/parked/stg.example.com/attach", `{"app":"api","environment":"prod"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("attach env = %d %s", rec.Code, rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/move", `{"domain":"x.example.com","from":"web","from_environment":"stg","to":"web","to_environment":"prod"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "requires_confirmation") {
		t.Fatalf("unconfirmed env move = %d %s", rec.Code, rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/move", `{"domain":"x.example.com","from":"web","from_environment":"stg","to":"web","to_environment":"prod","confirm":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("env move = %d %s", rec.Code, rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/park", `{"app":"web","domain":"x.example.com","environment":"nope"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown env = %d, want 404", rec.Code)
	}
	if svc, _ := db.GetDesiredService(ctx, "web"); len(svc.Domains) != 1 {
		t.Fatalf("default list changed: %v", svc.Domains)
	}
}

func TestParkedDomainsAPI_StaticSites(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveStaticSite(ctx, store.StaticSite{Name: "docs", Domains: []string{"docs.example.com", "old.example.com"}, RootDir: "/srv/docs"}); err != nil {
		t.Fatal(err)
	}

	rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/park", `{"app":"docs","target_kind":"static_site","domain":"old.example.com"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"kind":"static_site"`) {
		t.Fatalf("park static = %d %s", rec.Code, rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodGet, "/api/v1/domains/parked", "")
	if !strings.Contains(rec.Body.String(), `"parked_from_kind":"static_site"`) {
		t.Fatalf("list = %s", rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/move", `{"domain":"docs.example.com","from":"docs","from_kind":"static_site","to":"web"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "requires_confirmation") {
		t.Fatalf("unconfirmed move = %d %s", rec.Code, rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/move", `{"domain":"docs.example.com","from":"docs","from_kind":"static_site","to":"web","confirm":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("move static to app = %d %s", rec.Code, rec.Body.String())
	}
	if svc, _ := db.GetDesiredService(ctx, "web"); len(svc.Domains) != 1 || svc.Domains[0] != "docs.example.com" {
		t.Fatalf("web domains = %v", svc.Domains)
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/parked/old.example.com/attach", `{"app":"docs","target_kind":"static_site"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("attach to static = %d %s", rec.Code, rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/park", `{"app":"docs","target_kind":"bogus","domain":"old.example.com"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad kind = %d", rec.Code)
	}
}
