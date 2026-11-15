package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func fixedConnectionsNow(t *testing.T) time.Time {
	t.Helper()
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	prev := connectionsNow
	connectionsNow = func() time.Time { return now }
	t.Cleanup(func() { connectionsNow = prev })
	return now
}

func connectionsServer(t *testing.T, gotURL *string, body any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotURL = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func TestRun_ConnectionsList(t *testing.T) {
	now := fixedConnectionsNow(t)
	soon, far := now.Add(30*time.Hour), now.Add(10*24*time.Hour+time.Hour)
	var got string
	srv := connectionsServer(t, &got, map[string]any{"connections": []apiclient.Connection{
		{Ref: "git:gh1", Kind: "git", Provider: "github", Label: "Org GitHub", ScopeType: "instance", Health: "ok", ExpiresAt: &far, BoundCount: 3},
		{Ref: "dns:cf", Kind: "dns", Provider: "cloudflare", Label: "CF", ScopeType: "org", ScopeID: "acme", Health: "expiring", ExpiresAt: &soon},
	}})
	defer srv.Close()

	out, _ := runCLIExpectOK(t, []string{"connections", "list", "--api-url", srv.URL})
	if got != "/api/v1/connections" {
		t.Errorf("url = %q", got)
	}
	for _, want := range []string{"REF", "git:gh1", "in 10 days", "in 30 hours", "org:acme", "3"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q: %q", want, out)
		}
	}
}

func TestRun_ConnectionsListFiltersAndJSON(t *testing.T) {
	var got string
	srv := connectionsServer(t, &got, map[string]any{"connections": []apiclient.Connection{{Ref: "git:gh1", Kind: "git"}}})
	defer srv.Close()

	out, _ := runCLIExpectOK(t, []string{"connections", "list", "--kind", "git", "--scope", "org", "--health", "ok", "--json", "--api-url", srv.URL})
	if got != "/api/v1/connections?health=ok&kind=git&scope=org" {
		t.Errorf("url = %q", got)
	}
	var list []apiclient.Connection
	if err := json.Unmarshal([]byte(out), &list); err != nil || len(list) != 1 || list[0].Ref != "git:gh1" {
		t.Errorf("json = %q err=%v", out, err)
	}
}

func TestRun_ConnectionsListEmpty(t *testing.T) {
	for name, body := range map[string]any{"null": map[string]any{"connections": nil}, "empty": map[string]any{"connections": []any{}}} {
		t.Run(name, func(t *testing.T) {
			var got string
			srv := connectionsServer(t, &got, body)
			defer srv.Close()
			out, _ := runCLIExpectOK(t, []string{"connections", "list", "--api-url", srv.URL})
			if strings.TrimSpace(out) != "No connections" {
				t.Errorf("stdout = %q", out)
			}
			js, _ := runCLIExpectOK(t, []string{"connections", "list", "--json", "--api-url", srv.URL})
			if strings.TrimSpace(js) != "[]" {
				t.Errorf("json = %q, want []", js)
			}
		})
	}
}

func TestRun_ConnectionsShow(t *testing.T) {
	fixedConnectionsNow(t)
	var got string
	srv := connectionsServer(t, &got, apiclient.Connection{Ref: "git:gh1", Kind: "git", Provider: "github", Label: "Org GitHub", ScopeType: "instance", AuthMethod: "app", Health: "ok", Capabilities: []string{"clone", "webhook"}, BoundCount: 2})
	defer srv.Close()

	out, _ := runCLIExpectOK(t, []string{"connections", "show", "git:gh1", "--api-url", srv.URL})
	if got != "/api/v1/connections/git:gh1" {
		t.Errorf("url = %q", got)
	}
	for _, want := range []string{"Ref:", "git:gh1", "clone, webhook", "Bound:"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q: %q", want, out)
		}
	}
	js, _ := runCLIExpectOK(t, []string{"connections", "show", "git:gh1", "--json", "--api-url", srv.URL})
	var c apiclient.Connection
	if err := json.Unmarshal([]byte(js), &c); err != nil || c.Ref != "git:gh1" {
		t.Errorf("json = %q err=%v", js, err)
	}
}

func TestRun_ConnectionsShowNeedsRef(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run("levelrail-cli-test", []string{"connections", "show"}, &out, &errb, envMap()); code != exitUsage {
		t.Errorf("exit = %d, want %d", code, exitUsage)
	}
}

func TestRun_ConnectionsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"connection not found"}`))
	}))
	defer srv.Close()
	stderr := runCLIExpectAPIError(t, []string{"connections", "show", "nope", "--api-url", srv.URL})
	if !strings.Contains(stderr, "connection not found") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRun_ConnectionsCatalog(t *testing.T) {
	var got string
	srv := connectionsServer(t, &got, map[string]any{"catalog": []apiclient.ConnectionCatalogEntry{{ID: "github", Category: "git", Name: "GitHub", ConnectMethods: []string{"app", "token"}}}})
	defer srv.Close()
	out, _ := runCLIExpectOK(t, []string{"connections", "catalog", "--api-url", srv.URL})
	if got != "/api/v1/connections/catalog" || !strings.Contains(out, "GitHub") || !strings.Contains(out, "app, token") {
		t.Errorf("url=%q stdout=%q", got, out)
	}
}
