package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
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

func wwwNoProviderRouter(t *testing.T, addrs map[string][]string) (*Router, *http.Cookie) {
	t.Helper()
	var calls atomic.Int32
	rt, cookie := cutoverRouter(t, false, &calls)
	rt.domainAuto.resolveTarget = func(context.Context, string) (*dnsTarget, error) { return nil, nil }
	rt.publicHost = "203.0.113.7"
	rt.detectPublicIPs = func(context.Context) []string { return nil }
	rt.lookupHost = func(_ context.Context, host string) ([]string, error) {
		if a, ok := addrs[host]; ok {
			return a, nil
		}
		return nil, errors.New("no such host")
	}
	return rt, cookie
}

func TestWWWPlanLookupMarksRecordsAlreadyCorrect(t *testing.T) {
	rt, cookie := wwwNoProviderRouter(t, map[string][]string{
		"far.example.com":     {"203.0.113.7"},
		"www.far.example.com": {"203.0.113.7"},
	})
	var res wwwPlanResponse
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/domains/far.example.com/dns/plan", &res); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if !res.ApexPointsHere || !res.WWWPointsHere || res.WWWAttached {
		t.Fatalf("facts = %+v", res)
	}
	for _, rec := range res.Records {
		if rec.State != wwwStateAlreadyRight || len(rec.Current) != 1 {
			t.Fatalf("record = %+v", rec)
		}
	}
	if strings.Contains(res.Message, "empty zone") || res.Message == "" {
		t.Fatalf("message = %q", res.Message)
	}
}

func TestWWWPlanLookupMissingWWW(t *testing.T) {
	rt, cookie := wwwNoProviderRouter(t, map[string][]string{"far.example.com": {"203.0.113.7"}})
	var res wwwPlanResponse
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/domains/far.example.com/dns/plan", &res); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if !res.ApexPointsHere || res.WWWPointsHere {
		t.Fatalf("facts = %+v", res)
	}
	if res.Records[0].State != wwwStateAlreadyRight || res.Records[1].State != wwwStateMissing {
		t.Fatalf("records = %+v", res.Records)
	}
}
