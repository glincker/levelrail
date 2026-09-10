package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCheckDomainDNS(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/domains/app.example.com/check" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/domains/app.example.com/check", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.DomainCheckResource{
			Domain: "app.example.com", Status: "connected", Resolved: true,
			ExpectedHost: "203.0.113.10", ResolvedHosts: []string{"203.0.113.10"},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "check_domain_dns",
		Arguments: map[string]any{"name": "web", "domain": "app.example.com"},
	})
	if err != nil {
		t.Fatalf("CallTool(check_domain_dns) error = %v", err)
	}
	var out apiclient.DomainCheckResource
	decodeStructured(t, result, &out)
	if out.Status != "connected" {
		t.Errorf("out.Status = %q, want connected", out.Status)
	}
}

func TestCheckDomainDNS_NotResolving(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.DomainCheckResource{
			Domain: "app.example.com", Status: "not_resolving", Resolved: false, ExpectedHost: "203.0.113.10",
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "check_domain_dns",
		Arguments: map[string]any{"name": "web", "domain": "app.example.com"},
	})
	if err != nil {
		t.Fatalf("CallTool(check_domain_dns) error = %v", err)
	}
	var out apiclient.DomainCheckResource
	decodeStructured(t, result, &out)
	if out.Status != "not_resolving" || out.Resolved {
		t.Errorf("out = %+v, want status=not_resolving resolved=false", out)
	}
}

func TestCheckDomainDNS_NotFound(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"app not found"}`))
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "check_domain_dns",
		Arguments: map[string]any{"name": "ghost", "domain": "app.example.com"},
	})
	if err != nil {
		t.Fatalf("CallTool(check_domain_dns) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for a 404 response")
	}
	if got := toolResultText(result); got == "" {
		t.Errorf("error text = %q, want a non-empty error message", got)
	}
}
