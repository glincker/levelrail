package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckReadonlyAbilities(t *testing.T) {
	tests := []struct {
		abilities []string
		wantErr   bool
	}{
		{[]string{"read"}, false},
		{[]string{"read", "read:sensitive"}, false},
		{[]string{"read", "write"}, true},
		{[]string{"write:sensitive"}, true},
		{[]string{"deploy"}, true},
		{[]string{"root"}, true},
		{nil, false},
	}
	for _, tc := range tests {
		t.Run(strings.Join(tc.abilities, ","), func(t *testing.T) {
			if err := checkReadonlyAbilities(tc.abilities); (err != nil) != tc.wantErr {
				t.Errorf("checkReadonlyAbilities(%v) error = %v, wantErr %v", tc.abilities, err, tc.wantErr)
			}
		})
	}
}

func whoamiServer(t *testing.T, status int, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/whoami" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestRun_ReadonlyRefusesTokenThatCanWrite(t *testing.T) {
	noEnv := func(string) (string, bool) { return "", false }
	logger := slog.New(slog.DiscardHandler)
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"root token", http.StatusOK, `{"kind":"token","abilities":["root"]}`, "refusing to start"},
		{"deploy token", http.StatusOK, `{"kind":"token","abilities":["read","deploy"]}`, "deploy"},
		{"whoami fails", http.StatusUnauthorized, `{"error":"invalid token"}`, "--readonly"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url := whoamiServer(t, tc.status, tc.body)
			err := run("levelrail-mcp", []string{"--readonly", "--token", "t", "--api-url", url}, noEnv, logger)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("run() error = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}
