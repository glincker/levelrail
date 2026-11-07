package ingress

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStateOfTable(t *testing.T) {
	cases := []struct {
		name string
		f    ServiceFacts
		want HostState
	}{
		{"building wins over stopped", ServiceFacts{Building: true, Stopped: true}, StateBuilding},
		{"stopped", ServiceFacts{Stopped: true, Containers: 1}, StateStopped},
		{"no containers yet", ServiceFacts{}, StateStarting},
		{"fresh running", ServiceFacts{Containers: 1, Running: 1, Fresh: true}, StateStarting},
		{"exited only", ServiceFacts{Containers: 1}, StateUnhealthy},
		{"remote node", ServiceFacts{Remote: true}, StateUnknown},
		{"old running yet unrouted", ServiceFacts{Containers: 1, Running: 1}, StateUnknown},
	}
	for _, c := range cases {
		if got := StateOf(c.f); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func TestSnapshotLookupUnattachedAndAttached(t *testing.T) {
	s := NewDiagnosticSnapshot(func(context.Context) ([]ServiceFacts, error) {
		return []ServiceFacts{{Name: "web", Domains: []string{"Web.Example.com", "*.wild.test"}, Stopped: true}}, nil
	})
	if got := s.Lookup(context.Background(), "web.example.com"); got.State != StateNotLoaded {
		t.Fatalf("before refresh: %v", got)
	}
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := s.Lookup(context.Background(), "web.example.com"); got.State != StateStopped {
		t.Fatalf("attached: %v", got)
	}
	if got := s.Lookup(context.Background(), "WEB.example.com.:443"); got.State != StateStopped {
		t.Fatalf("normalized: %v", got)
	}
	if got := s.Lookup(context.Background(), "other.example.com"); got.State != StateUnattached {
		t.Fatalf("unattached leaked: %v", got)
	}
	for host, want := range map[string]HostState{"a.wild.test": StateStopped, "a.b.wild.test": StateUnattached, "wild.test": StateUnattached} {
		if got := s.Lookup(context.Background(), host).State; got != want {
			t.Errorf("wildcard %s: got %s want %s", host, got, want)
		}
	}
}

func TestCatchAllFallsBackToStaticWhenEndpointDown(t *testing.T) {
	h := &Hardening{BrandName: "B", DocsURL: "https://d", DiagnosticDial: "127.0.0.1:1", DiagnosticToken: "tok"}
	raw, err := json.Marshal(diagnosticCatchAll(h))
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)
	if !strings.Contains(js, `"reverse_proxy"`) || !strings.Contains(js, `"errors"`) || !strings.Contains(js, `"static_response"`) {
		t.Fatalf("proxy hop needs an errors fallback to the static page: %s", js)
	}
	bare, _ := json.Marshal(diagnosticCatchAll(&Hardening{BrandName: "B"}))
	if strings.Contains(string(bare), "reverse_proxy") {
		t.Fatal("no dial must keep the static page only")
	}
}

func TestTrustedDiagnosticIgnoresClientHeaders(t *testing.T) {
	var gotHost string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { gotHost = r.Host })
	h := TrustedDiagnosticHandler("secret", inner, "B", "https://d")

	req := httptest.NewRequest(http.MethodGet, DiagnosticPath, nil)
	req.Header.Set(DiagnosticHostHeader, "evil.example.com")
	req.Header.Set(DiagnosticTokenHeader, "guess")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound || gotHost != "" || !strings.Contains(rec.Body.String(), "No app is serving this address") {
		t.Fatalf("client supplied headers were trusted: %d %q", rec.Code, gotHost)
	}

	req = httptest.NewRequest(http.MethodGet, DiagnosticPath, nil)
	req.Header.Set(DiagnosticHostHeader, "app.example.com")
	req.Header.Set(DiagnosticTokenHeader, "secret")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if gotHost != "app.example.com" {
		t.Fatalf("ingress host not used: %q", gotHost)
	}
}

func edgeRequest(h http.Handler, host, accept string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = host
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func edgeSnapshot(t *testing.T) *DiagnosticSnapshot {
	t.Helper()
	s := NewDiagnosticSnapshot(func(context.Context) ([]ServiceFacts, error) {
		return []ServiceFacts{
			{Name: "stopped-app", Domains: []string{"stopped.example.com"}, Stopped: true, Containers: 1},
			{Name: "build-app", Domains: []string{"build.example.com"}, Building: true},
			{Name: "sick-app", Domains: []string{"sick.example.com"}, Containers: 1},
		}, nil
	})
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestEdgeDiagnosticShowStateOffIsGeneric404(t *testing.T) {
	snap := edgeSnapshot(t)
	h := DiagnosticHandler(snap.Lookup, func(context.Context) bool { return false }, "B", "https://d")
	for _, host := range []string{"unknown.example.com", "stopped.example.com", "build.example.com"} {
		for _, accept := range []string{"", "text/html"} {
			rec := edgeRequest(h, host, accept)
			body := rec.Body.String()
			if rec.Code != http.StatusNotFound || strings.Contains(body, "stopped") || strings.Contains(body, "building") {
				t.Errorf("%s accept=%q: %d %q", host, accept, rec.Code, body)
			}
		}
	}
}

func TestEdgeDiagnosticShowStateOnByHost(t *testing.T) {
	snap := edgeSnapshot(t)
	h := DiagnosticHandler(snap.Lookup, func(context.Context) bool { return true }, "B", "https://d")
	cases := []struct {
		host string
		code int
		text string
	}{
		{"unknown.example.com", http.StatusNotFound, "not attached"},
		{"stopped.example.com", http.StatusServiceUnavailable, "The app is stopped"},
		{"build.example.com", http.StatusServiceUnavailable, "still building"},
		{"sick.example.com", http.StatusServiceUnavailable, "no healthy replica"},
	}
	for _, c := range cases {
		rec := edgeRequest(h, c.host+":80", "text/html")
		if rec.Code != c.code || !strings.Contains(rec.Body.String(), c.text) {
			t.Errorf("%s: %d, want %d with %q", c.host, rec.Code, c.code, c.text)
		}
		if strings.Contains(rec.Body.String(), "stopped-app") || strings.Contains(rec.Body.String(), "build-app") || strings.Contains(rec.Body.String(), "sick-app") {
			t.Errorf("%s: page leaks an app name", c.host)
		}
	}
}

func TestEdgePort80FallbackIsDiagnosticNot200(t *testing.T) {
	for _, redirect := range []bool{true, false} {
		srv := certRedirectServer(":80", &AutoHTTPSConfig{DisableRedir: true}, redirect, []string{"a.example.com"}, 443,
			&Hardening{BrandName: "B", DocsURL: "https://d", DiagnosticDial: "127.0.0.1:1", DiagnosticToken: "tok"})
		raw, _ := json.Marshal(srv)
		if !strings.Contains(string(raw), "reverse_proxy") || strings.Contains(string(raw), `"status_code":200`) {
			t.Errorf("redirect=%v: port 80 fallback must hop to the diagnostic endpoint: %s", redirect, raw)
		}
	}
}
