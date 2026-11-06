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
		return []ServiceFacts{{Name: "web", Domains: []string{"Web.Example.com"}, Stopped: true}}, nil
	})
	if got := s.Lookup(context.Background(), "web.example.com"); got.State != StateUnattached {
		t.Fatalf("before refresh: %v", got)
	}
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := s.Lookup(context.Background(), "web.example.com"); got.State != StateStopped || got.App != "web" {
		t.Fatalf("attached: %v", got)
	}
	if got := s.Lookup(context.Background(), "other.example.com"); got.State != StateUnattached || got.App != "" {
		t.Fatalf("unattached leaked: %v", got)
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
	h := TrustedDiagnosticHandler("secret", inner)

	req := httptest.NewRequest(http.MethodGet, DiagnosticPath, nil)
	req.Header.Set(DiagnosticHostHeader, "evil.example.com")
	req.Header.Set(DiagnosticTokenHeader, "guess")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound || gotHost != "" {
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
