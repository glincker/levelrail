package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// tokensFakeServer serves login, the token list and bulk revoke; it records requests.
type tokensFakeServer struct {
	*httptest.Server
	listQuery string
	bulk      []bulkRevokeRequest
	pruneBody []string
}

func newTokensFake(t *testing.T, match int, conflict bool) *tokensFakeServer {
	t.Helper()
	f := &tokensFakeServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session_token", Value: "s", Path: "/"}) //nolint:gosec // plain http test listener
		_, _ = w.Write([]byte(`{"username":"admin"}`))
	})
	mux.HandleFunc("GET /api/v1/auth/tokens", func(w http.ResponseWriter, r *http.Request) {
		f.listQuery = r.URL.RawQuery
		if r.URL.Query().Get("groups") == "name" {
			_, _ = w.Write([]byte(`{"groups":[{"name":"ci","count":3,"active":1,"expiring":1,"expired":2,"oldest_created":"2026-01-02T00:00:00Z"}],"total":3,"counts_by_status":{"active":1,"expiring":1,"expired":2,"revoked":0}}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"id":"t1","name":"ci","abilities":["read","write"],"created_at":"2026-01-02T00:00:00Z","status":"expired","expires_at":"2026-02-01T00:00:00Z"}],"total":1,"counts_by_status":{"active":0,"expiring":0,"expired":1,"revoked":0}}`))
	})
	mux.HandleFunc("POST /api/v1/tokens/bulk-revoke", func(w http.ResponseWriter, r *http.Request) {
		var req bulkRevokeRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.bulk = append(f.bulk, req)
		resp := bulkRevokeResponse{DryRun: req.DryRun, Count: match, Sample: []string{"ci", "old"}, Max: 500}
		if !req.DryRun {
			if conflict {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(resp)
				return
			}
			resp.Revoked = match
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("POST /api/v1/tokens/prune", func(w http.ResponseWriter, r *http.Request) {
		var body bytes.Buffer
		_, _ = body.ReadFrom(r.Body)
		f.pruneBody = append(f.pruneBody, strings.TrimSpace(body.String()))
		_, _ = w.Write([]byte(`{"dry_run":` + map[bool]string{true: "true", false: "false"}[strings.Contains(body.String(), `"dry_run":true`)] + `,"revoked":2,"expired":5}`))
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func runTokensCmd(f *tokensFakeServer, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	full := append([]string{"tokens"}, args...)
	full = append(full, "--api-url", f.URL)
	code := run("levelrail-cli-test", full, &stdout, &stderr, envMap())
	return code, stdout.String(), stderr.String()
}

func TestTokensLsFiltersAndGroups(t *testing.T) {
	f := newTokensFake(t, 0, false)
	code, out, errOut := runTokensCmd(f, "ls", "--status", "expired", "--name", "ci", "--kind", "agent", "--sort", "expires", "--limit", "5", "--token", "root-tok")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	for _, want := range []string{"status=expired", "name=ci", "kind=agent", "sort=expires", "limit=5"} {
		if !strings.Contains(f.listQuery, want) {
			t.Errorf("query %q lacks %q", f.listQuery, want)
		}
	}
	if !strings.Contains(out, "STATUS") || !strings.Contains(out, "read,write") || !strings.Contains(out, "expired") {
		t.Errorf("table = %q", out)
	}
	_, out, _ = runTokensCmd(f, "ls", "--groups", "--token", "root-tok")
	if !strings.Contains(f.listQuery, "groups=name") || !strings.Contains(out, "EXPIRING") || !strings.Contains(out, "ci") {
		t.Errorf("groups query %q out %q", f.listQuery, out)
	}
}

func TestTokensRevokeBulkDryRunThenConfirm(t *testing.T) {
	f := newTokensFake(t, 7, false)
	code, out, errOut := runTokensCmd(f, "revoke", "--expired", "--yes", "--username", "admin", "--password", "x")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(f.bulk) != 2 || !f.bulk[0].DryRun || f.bulk[1].DryRun {
		t.Fatalf("want dry run then real call, got %+v", f.bulk)
	}
	if f.bulk[1].ConfirmCount == nil || *f.bulk[1].ConfirmCount != 7 || f.bulk[1].Filter == nil || f.bulk[1].Filter.Status != "expired" {
		t.Errorf("confirm call = %+v", f.bulk[1])
	}
	if !strings.Contains(out, "revoked 7 tokens") || !strings.Contains(errOut, "7 tokens will be revoked: ci, old") {
		t.Errorf("out %q err %q", out, errOut)
	}
}

func TestTokensRevokeBulkDryRunOnly(t *testing.T) {
	f := newTokensFake(t, 7, false)
	code, out, _ := runTokensCmd(f, "revoke", "--kind", "emergency", "--dry-run", "--username", "admin", "--password", "x")
	if code != exitOK || len(f.bulk) != 1 || !strings.Contains(out, "7 tokens would be revoked: ci, old") {
		t.Errorf("code %d calls %d out %q", code, len(f.bulk), out)
	}
}

func TestTokensRevokeBulkConfirmCountMismatch(t *testing.T) {
	f := newTokensFake(t, 3, true)
	code, _, errOut := runTokensCmd(f, "revoke", "--name", "ci", "--yes", "--username", "admin", "--password", "x")
	if code == exitOK || !strings.Contains(errOut, "changed since the preview") {
		t.Errorf("code %d err %q", code, errOut)
	}
}

func TestTokensRevokeBulkRefusesAPIToken(t *testing.T) {
	f := newTokensFake(t, 3, false)
	code, _, errOut := runTokensCmd(f, "revoke", "--expired", "--yes", "--token", "api-tok")
	if code != exitValidation || !strings.Contains(errOut, bulkRevokeAPITokenMessage) || len(f.bulk) != 0 {
		t.Errorf("code %d err %q calls %d", code, errOut, len(f.bulk))
	}
}

func TestTokensRevokeBulkNeedsConfirmation(t *testing.T) {
	f := newTokensFake(t, 3, false)
	code, _, errOut := runTokensCmd(f, "revoke", "id1", "id2", "--username", "admin", "--password", "x")
	if code == exitOK || !strings.Contains(errOut, "--yes") || len(f.bulk) != 1 {
		t.Errorf("code %d err %q calls %d", code, errOut, len(f.bulk))
	}
}

func TestTokensPrune(t *testing.T) {
	f := newTokensFake(t, 0, false)
	code, out, errOut := runTokensCmd(f, "prune", "--older-than", "30d", "--yes", "--token", "root-tok")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(f.pruneBody) != 2 || !strings.Contains(f.pruneBody[0], `"dry_run":true`) || !strings.Contains(f.pruneBody[1], `"older_than_days":30`) || strings.Contains(f.pruneBody[1], `"dry_run":true`) {
		t.Errorf("bodies = %v", f.pruneBody)
	}
	if !strings.Contains(out, "deleted 2 revoked and 5 expired") {
		t.Errorf("out = %q", out)
	}
	f.pruneBody = nil
	_, out, _ = runTokensCmd(f, "prune", "--dry-run", "--token", "root-tok")
	if len(f.pruneBody) != 1 || !strings.Contains(out, "would delete 2 revoked and 5 expired") {
		t.Errorf("dry run bodies %v out %q", f.pruneBody, out)
	}
	if code, _, _ := runTokensCmd(f, "prune", "--older-than", "soon", "--token", "root-tok"); code != exitValidation {
		t.Errorf("bad window exit = %d", code)
	}
}
