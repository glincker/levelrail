package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_BackupsCoverage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/backups/coverage" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"summary":{"total":2,"protected":1,"verified":1,"unprotected":1},"resources":[
			{"kind":"database","name":"orders","state":"unprotected","primary_action":"protect","reason":"no backup schedule"},
			{"kind":"volume","app_name":"web","name":"uploads","state":"verified","primary_action":"restore"}],
			"targets":[],"defaults":{"schedule":"0 3 * * *","retain":7}}`)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"backups", "coverage", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("exit = %d (stderr=%q)", got, stderr.String())
	}
	for _, want := range []string{"1 of 2 resources protected", "orders", "web/uploads", "no backup schedule"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout.String())
		}
	}

	stdout.Reset()
	if got := run("levelrail-cli-test", []string{"backups", "coverage", "--unprotected", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("exit = %d", got)
	}
	if strings.Contains(stdout.String(), "web/uploads") {
		t.Errorf("--unprotected listed a verified resource:\n%s", stdout.String())
	}
}

func TestRun_BackupsProtectAll_PlansUnlessApplied(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		applied, _ := body["apply"].(bool)
		resp := map[string]any{"applied": applied, "target_id": "t1", "schedule": "0 3 * * *", "retain": 7, "plan": []map[string]any{{"kind": "database", "name": "orders"}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"backups", "protect-all", "--target", "t1", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("exit = %d (stderr=%q)", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "would schedule 1 resources") || !strings.Contains(stdout.String(), "nothing changed") {
		t.Errorf("plan output = %q", stdout.String())
	}
	if bodies[0]["apply"] != false {
		t.Errorf("default must not apply, body = %v", bodies[0])
	}

	stdout.Reset()
	if got := run("levelrail-cli-test", []string{"backups", "protect-all", "--target", "t1", "--apply", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("exit = %d", got)
	}
	if bodies[1]["apply"] != true || !strings.Contains(stdout.String(), "scheduled 1 resources") {
		t.Errorf("apply body = %v, output = %q", bodies[1], stdout.String())
	}
}

func TestRun_BackupsProtectAll_RequiresTarget(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "protect-all", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got == exitOK {
		t.Fatalf("exit = %d, want failure without --target", got)
	}
}

func TestRun_DeleteDataNeedsTypedConfirm(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	for _, cmd := range []string{"apps", "databases"} {
		gotQuery = ""
		var stdout, stderr bytes.Buffer
		if got := run("levelrail-cli-test", []string{cmd, "delete", "web", "--delete-data", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got == exitOK {
			t.Fatalf("%s delete --delete-data without --confirm must fail", cmd)
		}
		if gotQuery != "" {
			t.Fatalf("%s: request sent without confirmation: %q", cmd, gotQuery)
		}
		if got := run("levelrail-cli-test", []string{cmd, "delete", "web", "--delete-data", "--confirm", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitOK {
			t.Fatalf("%s delete confirmed: exit = %d (stderr=%q)", cmd, got, stderr.String())
		}
		if !strings.Contains(gotQuery, "delete_data=true") || !strings.Contains(gotQuery, "confirm=web") {
			t.Fatalf("%s: query = %q", cmd, gotQuery)
		}
	}
}

func TestRun_BackupsOrphans(t *testing.T) {
	var deleted string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/storage/orphans":
			_, _ = io.WriteString(w, `{"orphans":[{"id":"db-gone-data","name":"db-gone-data","size_bytes":4096,"age_days":3,"belonged_to":{"kind":"database","name":"gone","role":"data"},"backup_count":0}],"total_bytes":4096,"threshold_bytes":1073741824}`)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/storage/orphans/"):
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			deleted = r.URL.Path + "|" + body["confirm"]
			_, _ = io.WriteString(w, `{"removed":"db-gone-data","reclaimed_bytes":4096}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"backups", "orphans", "list", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("list exit = %d (stderr=%q)", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "db-gone-data") || !strings.Contains(stdout.String(), "gone") {
		t.Errorf("list output = %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if got := run("levelrail-cli-test", []string{"backups", "orphans", "reclaim", "db-gone-data", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got == exitOK {
		t.Fatal("reclaim without --confirm must fail")
	}
	if deleted != "" {
		t.Fatalf("reclaim without --confirm reached the server: %s", deleted)
	}

	stdout.Reset()
	if got := run("levelrail-cli-test", []string{"backups", "orphans", "reclaim", "db-gone-data", "--confirm", "db-gone-data", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("reclaim exit = %d (stderr=%q)", got, stderr.String())
	}
	if deleted != "/api/v1/storage/orphans/db-gone-data|db-gone-data" {
		t.Errorf("server saw %q", deleted)
	}
}
