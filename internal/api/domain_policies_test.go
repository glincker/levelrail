package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type policyDNS map[string][]netip.Addr

func (f policyDNS) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := f[host]; ok {
		return a, nil
	}
	return nil, errors.New("no such host")
}

func doPolicy(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func TestDomainPolicies_HeadersRoundTrip(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)
	const path = "/api/v1/apps/web/domains/app.example.com/headers"

	rec := doPolicy(t, rt, cookie, http.MethodGet, path, "")
	var got domainPolicyResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 || got.Configured || string(got.Spec) != `{"rules":[]}` {
		t.Fatalf("default GET = %d %s", rec.Code, rec.Body.String())
	}
	body := `{"rules":[{"side":"response","op":"set","name":"X-Frame-Options","value":"DENY"}],"hide_server":true}`
	if rec := doPolicy(t, rt, cookie, http.MethodPut, path, body); rec.Code != 200 {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body.String())
	}
	row, found, err := db.GetDomainTrafficPolicy(context.Background(), "app.example.com", "headers")
	if err != nil || !found || !strings.Contains(string(row.Spec), "X-Frame-Options") {
		t.Fatalf("stored = %+v %v %v", row, found, err)
	}
	if rec := doPolicy(t, rt, cookie, http.MethodDelete, path, ""); rec.Code != 200 {
		t.Fatalf("DELETE = %d", rec.Code)
	}
	if _, found, _ := db.GetDomainTrafficPolicy(context.Background(), "app.example.com", "headers"); found {
		t.Fatal("DELETE left the row")
	}
}

func TestDomainPolicies_Validation(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomains(t, db, "app.example.com")
	cookie := loginTestSession(t, rt, db)
	rt.domainPolicy.resolver = policyDNS{
		"public.example.org":  {netip.MustParseAddr("93.184.216.34")},
		"private.example.org": {netip.MustParseAddr("10.1.2.3")},
	}
	base := "/api/v1/apps/web/domains/app.example.com/"
	tests := []struct {
		name, kind, body string
		status           int
		field            string
	}{
		{"header injection", "headers", `{"rules":[{"side":"response","op":"set","name":"X-A","value":"a\r\nSet-Cookie: x"}]}`, 400, "rules[0].value"},
		{"host header refused", "headers", `{"rules":[{"side":"request","op":"set","name":"Host","value":"x"}]}`, 400, "rules[0].name"},
		{"hsts without real tls", "headers", `{"rules":[],"security":{"hsts":true}}`, 400, "security.hsts"},
		{"unknown field", "headers", `{"rulez":[]}`, 400, ""},
		{"loopback forward refused", "forwarders", `{"rules":[{"match":{"kind":"prefix","path":"/x"},"action":"url","url":"http://127.0.0.1:2019"}]}`, 400, "rules[0].url"},
		{"metadata forward refused", "forwarders", `{"rules":[{"match":{"kind":"prefix","path":"/x"},"action":"url","url":"http://169.254.169.254/latest"}]}`, 400, "rules[0].url"},
		{"private name refused", "forwarders", `{"rules":[{"match":{"kind":"prefix","path":"/x"},"action":"url","url":"https://private.example.org"}]}`, 400, "rules[0].url"},
		{"public name allowed", "forwarders", `{"rules":[{"match":{"kind":"prefix","path":"/x"},"action":"url","url":"https://public.example.org/base"}]}`, 200, ""},
		{"self loop refused", "forwarders", `{"rules":[{"match":{"kind":"prefix","path":"/x"},"action":"url","url":"https://app.example.com/y"}]}`, 400, "rules[0].url"},
		{"unknown app refused", "forwarders", `{"rules":[{"match":{"kind":"prefix","path":"/x"},"action":"app","app":"ghost"}]}`, 400, "rules[0].app"},
		{"app forward ok", "forwarders", `{"rules":[{"match":{"kind":"prefix","path":"/x"},"action":"app","app":"web"}]}`, 200, ""},
		{"geo ok", "geo", `{"mode":"deny","countries":["RU"],"action":"block"}`, 200, ""},
		{"geo bad code", "geo", `{"mode":"deny","countries":["XX"],"action":"block"}`, 400, "countries[0]"},
		{"cache post refused", "cache", `{"enabled":true,"rules":[{"match":{"kind":"prefix","path":"/","methods":["POST"]},"ttl_seconds":5}]}`, 400, "rules[0].match.methods[0]"},
		{"cache ok", "cache", `{"enabled":true,"rules":[{"match":{"kind":"prefix","path":"/assets"},"ttl_seconds":60}]}`, 200, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doPolicy(t, rt, cookie, http.MethodPut, base+tt.kind, tt.body)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.status, rec.Body.String())
			}
			if tt.field == "" {
				return
			}
			var res policyValidationResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
				t.Fatal(err)
			}
			for _, f := range res.Fields {
				if f.Field == tt.field {
					return
				}
			}
			t.Fatalf("fields %+v missing %s", res.Fields, tt.field)
		})
	}
}

func TestDomainPolicies_Abilities(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	const plaintext = "policy-read-token" //nolint:gosec // fake fixture
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_policy_ro", Name: "reader", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/apps/web/domains/app.example.com/"
	tests := []struct {
		method, path, body string
		status             int
	}{
		{http.MethodGet, base + "headers", "", 200},
		{http.MethodGet, base + "policies", "", 200},
		{http.MethodGet, base + "redirects", "", 200},
		{http.MethodGet, base + "ports", "", 200},
		{http.MethodPost, base + "policies/preview", `{"path":"/"}`, 200},
		{http.MethodPut, base + "headers", `{"rules":[]}`, 403},
		{http.MethodDelete, base + "geo", "", 403},
		{http.MethodPost, base + "cache/purge", `{"scope":"all"}`, 403},
		{http.MethodPut, base + "redirects", `{"force_https":true}`, 403},
		{http.MethodPost, base + "redirects/canonical", `{"preset":"both"}`, 403},
		{http.MethodPut, base + "ports/9000/restrict", `{"sources":["1.2.3.4"]}`, 403},
		{http.MethodGet, "/api/v1/system/geoip?ip=8.8.8.8", "", 200},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer "+plaintext)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.status, rec.Body.String())
			}
		})
	}
}

func TestDomainPolicies_SummaryPreviewPurgeGeo(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)
	base := "/api/v1/apps/web/domains/app.example.com/"
	if rec := doPolicy(t, rt, cookie, http.MethodPut, base+"cache", `{"enabled":true,"rules":[{"match":{"kind":"prefix","path":"/assets"},"ttl_seconds":60}]}`); rec.Code != 200 {
		t.Fatalf("PUT cache = %d", rec.Code)
	}
	rec := doPolicy(t, rt, cookie, http.MethodGet, base+"policies", "")
	var sum domainPoliciesResource
	if err := json.Unmarshal(rec.Body.Bytes(), &sum); err != nil || sum.Policy.Cache == nil || sum.Limits.MaxHeaderRules == 0 || len(sum.Cache.Series) == 0 || sum.UpdatedAt["cache"] == "" {
		t.Fatalf("summary = %d %s", rec.Code, rec.Body.String())
	}
	rec = doPolicy(t, rt, cookie, http.MethodPost, base+"policies/preview", `{"policy":{"forwarders":{"rules":[{"match":{"kind":"prefix","path":"/api"},"action":"app","app":"web"}]}},"path":"/api/x","urls":["http://app.example.com/a"]}`)
	var pre policyPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &pre); err != nil || rec.Code != 200 || len(pre.Hops) != 1 || !strings.Contains(pre.Steps[len(pre.Steps)-1].Outcome, "app web") {
		t.Fatalf("preview = %d %s", rec.Code, rec.Body.String())
	}
	rec = doPolicy(t, rt, cookie, http.MethodPost, base+"policies/preview", `{"policy":{"geo":{"mode":"x"}}}`)
	if err := json.Unmarshal(rec.Body.Bytes(), &pre); err != nil || len(pre.Errors) == 0 || !strings.HasPrefix(pre.Errors[0].Field, "geo.") {
		t.Fatalf("preview errors = %s", rec.Body.String())
	}
	for body, want := range map[string]int{`{"scope":"all"}`: 200, `{"scope":"url","value":"/a?b=1"}`: 200, `{"scope":"prefix","value":"no-slash"}`: 400, `{"scope":"everything"}`: 400} {
		if rec := doPolicy(t, rt, cookie, http.MethodPost, base+"cache/purge", body); rec.Code != want {
			t.Errorf("purge %s = %d, want %d", body, rec.Code, want)
		}
	}
	if rec := doPolicy(t, rt, cookie, http.MethodGet, base+"cache/stats", ""); rec.Code != 200 {
		t.Errorf("stats = %d", rec.Code)
	}
	rec = doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/system/geoip?ip=10.0.0.1", "")
	var geo geoLookupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &geo); err != nil || !geo.Private {
		t.Fatalf("geoip = %d %s", rec.Code, rec.Body.String())
	}
	if rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/system/geoip?ip=nope", ""); rec.Code != 400 {
		t.Errorf("bad ip = %d", rec.Code)
	}
	if rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/domains/other.example.com/policies", ""); rec.Code != 404 {
		t.Errorf("foreign domain = %d", rec.Code)
	}
}

func TestDomainPolicies_Redirects(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomains(t, db, "example.com", "old.example.com", "legacy.example.com")
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	base := "/api/v1/apps/web/domains/example.com/redirects"

	if rec := doPolicy(t, rt, cookie, http.MethodPut, base, `{"force_https":true,"force_https_status":302}`); rec.Code != 400 {
		t.Fatalf("bad status = %d", rec.Code)
	}
	rec := doPolicy(t, rt, cookie, http.MethodPut, base, `{"force_https":true,"trailing_slash":"remove"}`)
	var res domainRedirectsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || rec.Code != 200 || !res.Settings.ForceHTTPS || len(res.Samples) != 3 || res.Effective.HandledBy == "" {
		t.Fatalf("PUT settings = %d %s", rec.Code, rec.Body.String())
	}

	rec = doPolicy(t, rt, cookie, http.MethodPost, base+"/canonical", `{"preset":"www-to-apex"}`)
	if rec.Code != 200 {
		t.Fatalf("canonical = %d %s", rec.Code, rec.Body.String())
	}
	svc, _ := db.GetDesiredService(ctx, "web")
	if !strings.Contains(strings.Join(svc.Domains, ","), "www.example.com") {
		t.Fatalf("counterpart not attached: %v", svc.Domains)
	}
	if r, found, _ := db.GetDomainRedirect(ctx, "www.example.com"); !found || r.TargetURL != "https://example.com" {
		t.Fatalf("www redirect = %+v %v", r, found)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.Canonical.Preset != "www-to-apex" {
		t.Fatalf("canonical state = %s", rec.Body.String())
	}
	if rec := doPolicy(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/domains/www.example.com/redirects/canonical", `{"preset":"apex-to-www"}`); rec.Code != 409 {
		t.Fatalf("flipping into a loop = %d %s, want 409", rec.Code, rec.Body.String())
	}
	if rec := doPolicy(t, rt, cookie, http.MethodPost, base+"/canonical", `{"preset":"both"}`); rec.Code != 200 {
		t.Fatalf("serve both = %d", rec.Code)
	}
	if _, found, _ := db.GetDomainRedirect(ctx, "www.example.com"); found {
		t.Fatal("serve both left the www redirect")
	}

	rec = doPolicy(t, rt, cookie, http.MethodPut, base+"/aliases", `{"aliases":["old.example.com","legacy.example.com"],"status_code":308}`)
	if rec.Code != 200 {
		t.Fatalf("aliases = %d %s", rec.Code, rec.Body.String())
	}
	if r, _, _ := db.GetDomainRedirect(ctx, "legacy.example.com"); r.StatusCode != 308 || r.TargetURL != "https://example.com" {
		t.Fatalf("alias row = %+v", r)
	}
	if rec := doPolicy(t, rt, cookie, http.MethodPut, base+"/aliases", `{"aliases":["old.example.com"]}`); rec.Code != 200 {
		t.Fatalf("shrink aliases = %d", rec.Code)
	}
	if _, found, _ := db.GetDomainRedirect(ctx, "legacy.example.com"); found {
		t.Fatal("dropped alias still redirects")
	}
	if rec := doPolicy(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/domains/old.example.com/redirects/aliases", `{"aliases":["example.com"]}`); rec.Code != 409 {
		t.Fatalf("primary that redirects itself = %d, want 409", rec.Code)
	}
	for body, want := range map[string]int{`{"aliases":["example.com"]}`: 400, `{"aliases":["nope.example.com"]}`: 400, `{"aliases":[],"status_code":303}`: 400} {
		if rec := doPolicy(t, rt, cookie, http.MethodPut, base+"/aliases", body); rec.Code != want {
			t.Errorf("aliases %s = %d, want %d", body, rec.Code, want)
		}
	}
	if rec := doPolicy(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/domains/example.com/redirect", `{"target_url":"https://old.example.com","status_code":307}`); rec.Code != 409 {
		t.Fatalf("legacy redirect into a loop = %d, want 409", rec.Code)
	}
}

func TestDomainPolicies_Ports(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveAppStream(ctx, store.AppStream{ID: "s1", ServiceName: "web", ContainerPort: 5432, HostPort: 15432, Protocol: "tcp", CreatedAt: "2026-10-10T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/apps/web/domains/app.example.com/ports"
	rec := doPolicy(t, rt, cookie, http.MethodGet, base, "")
	var res domainPortsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || len(res.Streams) != 1 || !res.Streams[0].OpenToAll {
		t.Fatalf("ports = %d %s", rec.Code, rec.Body.String())
	}
	if rec := doPolicy(t, rt, cookie, http.MethodPut, base+"/9999/restrict", `{"sources":["1.2.3.4"]}`); rec.Code != 404 {
		t.Fatalf("restrict unknown port = %d", rec.Code)
	}
	if rec := doPolicy(t, rt, cookie, http.MethodPut, base+"/15432/restrict", `{"sources":["not-an-ip"]}`); rec.Code != 400 {
		t.Fatalf("restrict bad source = %d", rec.Code)
	}
	for i := 0; i < 2; i++ {
		rec = doPolicy(t, rt, cookie, http.MethodPut, base+"/15432/restrict", `{"sources":["203.0.113.7","198.51.100.0/24"]}`)
		if rec.Code != 200 {
			t.Fatalf("restrict pass %d = %d %s", i, rec.Code, rec.Body.String())
		}
	}
	rules, _ := db.ListFirewallRules(ctx)
	if len(rules) != 3 {
		t.Fatalf("firewall rules = %+v, want 2 allows and 1 deny (idempotent)", rules)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.Streams[0].OpenToAll || len(res.Streams[0].AllowedSources) != 2 {
		t.Fatalf("restricted view = %s", rec.Body.String())
	}
	if rec := doPolicy(t, rt, cookie, http.MethodDelete, base+"/15432/restrict", ""); rec.Code != 200 {
		t.Fatalf("unrestrict = %d", rec.Code)
	}
	if rules, _ := db.ListFirewallRules(ctx); len(rules) != 0 {
		t.Fatalf("unrestrict left %+v", rules)
	}
}
