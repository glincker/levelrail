package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_ComposeBackups(t *testing.T) {
	const base = "/api/v1/apps/shop/compose-db-services"
	cases := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
		wantBody   string
		reply      string
		wantOut    string
	}{
		{"services", []string{"services", "shop"}, "GET", base, "", `[{"service":"db","engine":"postgres","scheduled":true,"schedule":"0 3 * * *","enabled":true}]`, "postgres"},
		{"schedule set", []string{"schedule", "set", "shop", "db", "--target", "t1", "--cron", "0 3 * * *", "--retain", "5"}, "PUT", base + "/db/backup-schedule", `{"target_id":"t1","schedule":"0 3 * * *","retain":5}`, `{"service":"db","schedule":"0 3 * * *","target_id":"t1","enabled":true}`, "set for shop/db"},
		{"schedule clear", []string{"schedule", "clear", "shop", "db"}, "DELETE", base + "/db/backup-schedule", "", ``, "kept"},
		{"runs", []string{"runs", "shop", "db"}, "GET", base + "/db/backups", "", `[{"id":"r1","status":"succeeded"}]`, "r1"},
		{"run", []string{"run", "shop", "db", "--target", "t1"}, "POST", base + "/db/backups", `{"target_id":"t1"}`, `{"id":"r2","status":"running"}`, "r2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var method, path, body string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				method, path, body = r.Method, r.URL.Path, strings.TrimSpace(string(b))
				if tc.reply == "" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.reply))
			}))
			defer srv.Close()
			out, _ := runCLIExpectOK(t, append(append([]string{"apps", "backups"}, tc.args...), "--api-url", srv.URL))
			if method != tc.wantMethod || path != tc.wantPath {
				t.Errorf("got %s %s, want %s %s", method, path, tc.wantMethod, tc.wantPath)
			}
			if body != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
			if !strings.Contains(out, tc.wantOut) {
				t.Errorf("stdout = %q, want %q", out, tc.wantOut)
			}
		})
	}

	t.Run("missing flags send nothing", func(t *testing.T) {
		hit := false
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
		defer srv.Close()
		for _, args := range [][]string{
			{"apps", "backups", "schedule", "set", "shop", "db", "--cron", "0 3 * * *"},
			{"apps", "backups", "schedule", "set", "shop", "db", "--target", "t1"},
			{"apps", "backups", "run", "shop", "db"},
		} {
			var so, se bytes.Buffer
			if got := run("levelrail-cli-test", append(args, "--api-url", srv.URL), &so, &se, envMap()); got != exitValidation {
				t.Errorf("%v exit = %d, want %d", args, got, exitValidation)
			}
		}
		if hit {
			t.Error("a request was sent despite missing flags")
		}
	})
}
