package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/standalone"
	"github.com/GLINCKER/levelrail/internal/store"
)

type staticResolver map[string][]netip.Addr

func (s staticResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := s[host]; ok {
		return a, nil
	}
	return nil, errors.New("no such host")
}

func newStandaloneRouter(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db := newTestRouter(t)
	rt.detectPublicIPs = func(context.Context) []string { return []string{"198.51.100.7"} }
	rt.interfaceAddrs = func() ([]net.Addr, error) { return nil, nil }
	rt.standaloneResolver = staticResolver{
		"metadata.internal": {netip.MustParseAddr("169.254.169.254")},
		"public.example":    {netip.MustParseAddr("203.0.113.9")},
	}
	rt.standaloneSafety = StandaloneSafety{
		ControlPorts: []standalone.ControlPort{{Port: 2019, Label: "ingress admin API", AlwaysBlock: true}, {Port: 8080, Label: "dashboard and API"}},
		IngressPorts: []int{8443},
	}
	return rt, db, loginTestSession(t, rt, db)
}

func sdReq(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func TestStandaloneDomainsAPI_Lifecycle(t *testing.T) {
	rt, db, cookie := newStandaloneRouter(t)

	rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/standalone", `{"domain":"Test.Example.com","target":"127.0.0.1:3000"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "needs_confirmation") {
		t.Fatalf("unconfirmed loopback = %d %s, want 409 asking for confirmation", rec.Code, rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/standalone", `{"domain":"Test.Example.com","target":"127.0.0.1:3000","confirm":true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var got standaloneDomainResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Domain != "test.example.com" || got.TargetURL != "http://127.0.0.1:3000" || !got.WebSocket || got.Scope != standalone.Scope || got.TargetKind != standalone.KindPort {
		t.Fatalf("created = %+v", got)
	}

	rec = sdReq(t, rt, cookie, http.MethodGet, "/api/v1/domains", "")
	if !strings.Contains(rec.Body.String(), `"kind":"standalone"`) || !strings.Contains(rec.Body.String(), `"target":"http://127.0.0.1:3000"`) {
		t.Fatalf("GET /domains = %s, want the standalone row", rec.Body.String())
	}

	rec = sdReq(t, rt, cookie, http.MethodPut, "/api/v1/apps/_standalone/domains/test.example.com/maintenance", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("per-domain maintenance through the standalone scope = %d %s", rec.Code, rec.Body.String())
	}
	if on, _ := db.GetDomainMaintenance(context.Background(), "test.example.com"); !on {
		t.Fatal("maintenance was not stored for the standalone domain")
	}
	if rec := sdReq(t, rt, cookie, http.MethodGet, "/api/v1/apps/_standalone/domains/other.example.com/maintenance", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown standalone domain = %d, want 404", rec.Code)
	}

	if rec := sdReq(t, rt, cookie, http.MethodPut, "/api/v1/domains/standalone/test.example.com", `{"websocket":false}`); rec.Code != http.StatusOK {
		t.Fatalf("an unchanged confirmed target must not ask again: %d %s", rec.Code, rec.Body.String())
	}
	if rec := sdReq(t, rt, cookie, http.MethodPut, "/api/v1/domains/standalone/test.example.com", `{"target":"10.0.0.9:80"}`); rec.Code != http.StatusConflict {
		t.Fatalf("a new private target must ask again: %d", rec.Code)
	}
	rec = sdReq(t, rt, cookie, http.MethodPut, "/api/v1/domains/standalone/test.example.com", `{"target":"https://public.example/base","read_timeout_seconds":60,"websocket":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.UpstreamTLS || got.HostHeader != standalone.HostUpstream || got.ReadTimeoutSeconds != 60 || !got.WebSocket {
		t.Fatalf("updated = %+v, want TLS from the URL, upstream host and the other fields kept", got)
	}

	if rec := sdReq(t, rt, cookie, http.MethodGet, "/api/v1/domains/standalone?domain=test.example.com", ""); rec.Code != http.StatusOK {
		t.Fatalf("get one = %d", rec.Code)
	}
	if rec := sdReq(t, rt, cookie, http.MethodDelete, "/api/v1/domains/standalone/test.example.com", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body.String())
	}
	if rec := sdReq(t, rt, cookie, http.MethodDelete, "/api/v1/domains/standalone/test.example.com", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d, want 404", rec.Code)
	}
	entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: "standalone_domain"})
	if err != nil || len(entries) < 3 {
		t.Fatalf("audit rows = %d, %v; want created, updated and deleted", len(entries), err)
	}
}

func TestStandaloneDomainsAPI_RefusesUnsafeTargets(t *testing.T) {
	rt, _, cookie := newStandaloneRouter(t)
	tests := []struct {
		name, body, want string
		status           int
	}{
		{"metadata ip", `{"domain":"a.example.com","target":"169.254.169.254:80","confirm":true}`, "metadata", http.StatusBadRequest},
		{"aws v6 metadata", `{"domain":"a.example.com","target":"[fd00:ec2::254]:80","confirm":true}`, "metadata", http.StatusBadRequest},
		{"metadata by name", `{"domain":"a.example.com","target":"http://metadata.internal","confirm":true}`, "metadata", http.StatusBadRequest},
		{"ingress admin api", `{"domain":"a.example.com","target":"2019","confirm":true}`, "admin API", http.StatusBadRequest},
		{"loop to own address", `{"domain":"a.example.com","target":"198.51.100.7:443","confirm":true}`, "ingress", http.StatusBadRequest},
		{"loop to local ingress", `{"domain":"a.example.com","target":"127.0.0.1:8443","confirm":true}`, "ingress", http.StatusBadRequest},
		{"url credentials", `{"domain":"a.example.com","target":"https://u:p@public.example"}`, "credentials", http.StatusBadRequest},
		{"bad port", `{"domain":"a.example.com","target":"70000"}`, "out of range", http.StatusBadRequest},
		{"bad domain", `{"domain":"localhost","target":"3000","confirm":true}`, "full hostname", http.StatusBadRequest},
		{"missing app", `{"domain":"a.example.com","target":"app:nope"}`, "does not exist", http.StatusBadRequest},
		{"control port unconfirmed", `{"domain":"a.example.com","target":"8080"}`, "dashboard and API", http.StatusConflict},
		{"unknown field", `{"domain":"a.example.com","target":"3000","bogus":1}`, "unknown field", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/standalone", tt.body)
			if rec.Code != tt.status || !strings.Contains(rec.Body.String(), tt.want) {
				t.Fatalf("= %d %s, want %d containing %q", rec.Code, rec.Body.String(), tt.status, tt.want)
			}
		})
	}
	if rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/standalone", `{"domain":"ok.example.com","target":"8080","confirm":true}`); rec.Code != http.StatusCreated {
		t.Fatalf("confirmed control port = %d %s, want 201", rec.Code, rec.Body.String())
	}
}

func TestStandaloneDomainsAPI_Conflicts(t *testing.T) {
	rt, db, cookie := newStandaloneRouter(t)
	seedAppWithDomain(t, db)
	if err := db.UpdateIngressSettings(context.Background(), store.IngressSettings{PrimaryDomain: "console.example.com"}); err != nil {
		t.Fatal(err)
	}
	for domain, owner := range map[string]string{"app.example.com": `app \"web\"`, "console.example.com": "the dashboard"} {
		rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/standalone", `{"domain":"`+domain+`","target":"https://public.example"}`)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), owner) {
			t.Fatalf("%s = %d %s, want 409 naming %s", domain, rec.Code, rec.Body.String(), owner)
		}
	}
	if rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/standalone", `{"domain":"tools.example.com","target":"https://public.example"}`); rec.Code != http.StatusCreated {
		t.Fatalf("public target = %d %s, want 201 without confirmation", rec.Code, rec.Body.String())
	}
	rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/standalone", `{"domain":"tools.example.com","target":"https://public.example"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "another standalone domain") {
		t.Fatalf("duplicate = %d %s", rec.Code, rec.Body.String())
	}
	rec = sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/standalone/validate", `{"domain":"app.example.com","target":"10.0.0.5:80"}`)
	var v standaloneValidation
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil || !strings.Contains(v.DomainError, "web") || !v.Assessment.Internal || len(v.Assessment.NeedsConfirmation) == 0 {
		t.Fatalf("validate = %s", rec.Body.String())
	}
}

func TestStandaloneDomainsAPI_Abilities(t *testing.T) {
	rt, db, _ := newStandaloneRouter(t)
	body := `{"domain":"x.example.com","target":"https://public.example"}`
	for _, tc := range []struct {
		abilities []string
		status    int
	}{
		{[]string{AbilityRead}, http.StatusForbidden},
		{[]string{AbilityWrite, AbilityDeploy}, http.StatusForbidden},
		{[]string{AbilityWriteSensitive}, http.StatusCreated},
	} {
		token := seedMatrixToken(t, db, "tok-"+strings.Join(tc.abilities, "-"), tc.abilities)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/domains/standalone", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("abilities %v = %d %s, want %d", tc.abilities, rec.Code, rec.Body.String(), tc.status)
		}
	}
}

func TestStandaloneDomainsAPI_ReservedAppName(t *testing.T) {
	rt, _, cookie := newStandaloneRouter(t)
	rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/apps", `{"name":"_standalone","image":"nginx:1","port":80}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "reserved") {
		t.Fatalf("reserved app name = %d %s", rec.Code, rec.Body.String())
	}
}

func TestServerAddressAPI(t *testing.T) {
	rt, _, cookie := newStandaloneRouter(t)
	rt.detectPublicIPs = func(context.Context) []string { return []string{"198.51.100.7", "2001:db8::7"} }
	rec := sdReq(t, rt, cookie, http.MethodGet, "/api/v1/system/server-address?domain=test.example.com", "")
	var got serverAddressResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.IPv4 != "198.51.100.7" || got.IPv6 != "2001:db8::7" || got.Source != serverAddressSourceDetected || !got.BehindNAT || len(got.Records) != 2 || got.Records[0].Type != "A" || got.Records[1].Type != "AAAA" {
		t.Fatalf("server address = %+v", got)
	}

	if rec := sdReq(t, rt, cookie, http.MethodPut, "/api/v1/system/server-address", `{"ipv4":"not-an-ip"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad override = %d", rec.Code)
	}
	rec = sdReq(t, rt, cookie, http.MethodPut, "/api/v1/system/server-address", `{"ipv4":"203.0.113.50"}`)
	got = serverAddressResource{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.IPv4 != "203.0.113.50" || got.IPv6 != "" || got.Source != serverAddressSourceOverride || got.Detected.IPv4 != "198.51.100.7" {
		t.Fatalf("after override = %+v", got)
	}
	if ips := rt.detectedPublicIPs(context.Background()); len(ips) != 1 || ips[0] != "203.0.113.50" {
		t.Fatalf("DNS checks must use the override, got %v", ips)
	}
	rt.interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("203.0.113.50"), Mask: net.CIDRMask(24, 32)}}, nil
	}
	rec = sdReq(t, rt, cookie, http.MethodGet, "/api/v1/system/server-address", "")
	got = serverAddressResource{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.BehindNAT {
		t.Fatalf("an address on a local interface is not NAT: %+v", got)
	}
}

func TestDNSRecordsFor(t *testing.T) {
	ips := serverAddressPair{IPv4: "203.0.113.4"}
	tests := []struct {
		domain, base, host string
		want               []string
	}{
		{"a.example.com", "", "", []string{"A a.example.com 203.0.113.4"}},
		{"a.example.com", "apps.example.net", "", []string{"A a.example.com 203.0.113.4", "CNAME a.example.com apps.example.net"}},
		{"x.apps.example.net", "apps.example.net", "", []string{"A x.apps.example.net 203.0.113.4"}},
		{"a.example.com", "", "edge.example.net", []string{"A a.example.com 203.0.113.4", "CNAME a.example.com edge.example.net"}},
		{"*.example.com", "apps.example.net", "", []string{"A *.example.com 203.0.113.4"}},
	}
	for _, tt := range tests {
		var got []string
		for _, r := range dnsRecordsFor(tt.domain, ips, tt.base, tt.host) {
			got = append(got, r.Type+" "+r.Name+" "+r.Value)
		}
		if strings.Join(got, "|") != strings.Join(tt.want, "|") {
			t.Errorf("dnsRecordsFor(%s) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}
