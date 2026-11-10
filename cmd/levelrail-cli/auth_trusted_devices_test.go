package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// recordingServer answers each "METHOD path" with a fixed status and body and
// records every request it saw.
func fakeRouteServer(t *testing.T, routes map[string]struct {
	status int
	body   string
}) (*httptest.Server, func() []string, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen, bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		mu.Lock()
		seen = append(seen, key)
		bodies = append(bodies, buf.String())
		mu.Unlock()
		rt, ok := routes[key]
		if !ok {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rt.status)
		_, _ = w.Write([]byte(rt.body))
	}))
	t.Cleanup(srv.Close)
	get := func(s *[]string) func() []string {
		return func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), *s...) }
	}
	return srv, get(&seen), get(&bodies)
}

func runCLIAuthMigrate(args []string) (int, string, string) {
	old := stdinSource
	stdinSource = strings.NewReader("")
	defer func() { stdinSource = old }()
	var out, errb bytes.Buffer
	code := run("levelrail-cli-test", args, &out, &errb, envMap())
	return code, out.String(), errb.String()
}

func TestAuthDevicesTrusted_ListAndRevoke(t *testing.T) {
	srv, seen, _ := fakeRouteServer(t, map[string]struct {
		status int
		body   string
	}{
		"GET /api/v1/auth/trusted-devices":         {200, `{"devices":[{"id":"td_1","label":"Firefox on Linux","ip":"10.0.0.2","current":true}]}`},
		"DELETE /api/v1/auth/trusted-devices/td_1": {204, ``},
	})
	base := []string{"--api-url", srv.URL, "--token", "t"}

	code, out, errb := runCLIAuthMigrate(append([]string{"auth", "devices", "trusted", "list"}, base...))
	if code != exitOK || !strings.Contains(out, "td_1") || !strings.Contains(out, "this browser") {
		t.Fatalf("list: code %d out %q err %q", code, out, errb)
	}
	if code, _, errb = runCLIAuthMigrate(append([]string{"auth", "devices", "trusted", "revoke", "td_1"}, base...)); code == exitOK || !strings.Contains(errb, "--yes") {
		t.Fatalf("revoke without --yes off a terminal must refuse: code %d err %q", code, errb)
	}
	if code, out, errb = runCLIAuthMigrate(append([]string{"auth", "devices", "trusted", "revoke", "td_1", "--yes"}, base...)); code != exitOK || !strings.Contains(out, "revoked") {
		t.Fatalf("revoke: code %d out %q err %q", code, out, errb)
	}
	want := []string{"GET /api/v1/auth/trusted-devices", "DELETE /api/v1/auth/trusted-devices/td_1"}
	if got := seen(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("requests = %v, want %v", got, want)
	}
}

func TestAuthSignInRequests_RevealPrintsOnceAndRespectsServer(t *testing.T) {
	srv, seen, _ := fakeRouteServer(t, map[string]struct {
		status int
		body   string
	}{
		"GET /api/v1/auth/sign-in-requests":                    {200, `{"codes":[{"id":"lc_1","revealable":true}],"approvals":[]}`},
		"POST /api/v1/auth/sign-in-requests/codes/lc_1/reveal": {200, `{"code":"482-913","expires_at":"2026-11-09T10:00:00Z"}`},
		"POST /api/v1/auth/sign-in-requests/codes/lc_2/reveal": {403, `{"error":"this token may not show sign-in codes"}`},
	})
	base := []string{"--api-url", srv.URL, "--token", "t"}

	code, out, errb := runCLIAuthMigrate(append([]string{"auth", "sign-in-requests", "list"}, base...))
	if code != exitOK || strings.Contains(out, "482-913") || !strings.Contains(out, "lc_1") {
		t.Fatalf("list must name the request without its code: code %d out %q err %q", code, out, errb)
	}
	code, out, errb = runCLIAuthMigrate(append([]string{"auth", "sign-in-requests", "reveal", "lc_1"}, base...))
	if code != exitOK || strings.Count(out+errb, "482-913") != 1 || strings.TrimSpace(out) != "482-913" {
		t.Fatalf("reveal must print the code exactly once on stdout: code %d out %q err %q", code, out, errb)
	}
	code, out, errb = runCLIAuthMigrate(append([]string{"auth", "sign-in-requests", "reveal", "lc_2"}, base...))
	if code == exitOK || out != "" || !strings.Contains(errb, "signin:approve") {
		t.Fatalf("a refused reveal must surface the server error and the token hint: code %d out %q err %q", code, out, errb)
	}
	want := "GET /api/v1/auth/sign-in-requests,POST /api/v1/auth/sign-in-requests/codes/lc_1/reveal,POST /api/v1/auth/sign-in-requests/codes/lc_2/reveal"
	if got := strings.Join(seen(), ","); got != want {
		t.Fatalf("requests = %s, want %s", got, want)
	}
}
