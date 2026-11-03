package api

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libdns/libdns"
)

func TestWWWPlanBothWithConflictingWWW(t *testing.T) {
	var calls atomic.Int32
	rt, cookie := cutoverRouter(t, false, &calls)
	mgr := &fakeDNSManager{recs: []libdns.Record{libdns.RR{Name: "www", Type: "A", Data: "198.51.100.9", TTL: time.Minute}}}
	rt.domainAuto.resolveTarget = func(context.Context, string) (*dnsTarget, error) {
		return &dnsTarget{Manager: mgr, Provider: "cloudflare", Zone: "far.example.com."}, nil
	}
	var res wwwPlanResponse
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/domains/far.example.com/dns/plan?mode=both&canonical=www", &res); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if len(res.Records) != 2 || res.Records[0].Name != "@" || res.Records[1].Name != "www" || res.Records[1].Type != "CNAME" {
		t.Fatalf("records = %+v", res.Records)
	}
	if res.Records[1].Outcome == "created" || len(res.Records[1].Replaces) != 1 {
		t.Fatalf("www should conflict with the existing A record: %+v", res.Records[1])
	}
	if res.Redirect == nil || res.Redirect.From != "far.example.com" || res.Redirect.To != "www.far.example.com" {
		t.Fatalf("redirect = %+v", res.Redirect)
	}
	if len(mgr.recs) != 1 {
		t.Fatalf("plan must not write, zone has %d records", len(mgr.recs))
	}
}

func TestWWWPlanRejectsNonApexAndBadMode(t *testing.T) {
	var calls atomic.Int32
	rt, cookie := cutoverRouter(t, false, &calls)
	rt.domainAuto.resolveTarget = func(context.Context, string) (*dnsTarget, error) {
		return &dnsTarget{Manager: &fakeDNSManager{}, Provider: "cloudflare", Zone: "example.com."}, nil
	}
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/domains/far.example.com/dns/plan", nil); code != http.StatusBadRequest {
		t.Fatalf("non apex status = %d, want 400", code)
	}
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/domains/far.example.com/dns/plan?mode=nope", nil); code != http.StatusBadRequest {
		t.Fatalf("bad mode status = %d, want 400", code)
	}
}
