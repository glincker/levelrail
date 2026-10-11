package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const analysisFixture = `{
 "repo_url":"https://github.com/acme/shop","branch":"main","monorepo":false,
 "name":{"value":"shop","evidence":"derived from the repository name shop"},
 "stack":{"runtime":"node","framework":"Next.js","evidence":"next dependency found"},
 "build":{"type":"railpack","evidence":"no Dockerfile found","alternatives":[]},
 "port":{"value":3000,"evidence":"Next.js default port"},
 "health":{"value":"/","evidence":"Next.js default health path"},
 "memory":{"bytes":805306368,"evidence":"768 MiB is a safe start"},
 "domain":{"value":"shop.apps.example.com","evidence":"from the apps base domain"},
 "env":[{"key":"DATABASE_URL","required":false,"secret":true,"source":".env.example","provided_by":"database:shop-postgres"},
        {"key":"NEXTAUTH_SECRET","required":true,"secret":true,"source":".env.example"}],
 "databases":[{"engine":"postgres","version":"16","suggested_name":"shop-postgres","env_var":"DATABASE_URL","field":"url","evidence":"DATABASE_URL declared in .env.example"}],
 "warnings":[],"files_read":3}`

func newAnalyzeServer(t *testing.T, calls *[]string, bodies map[string][]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		key := r.Method + " " + r.URL.Path
		*calls = append(*calls, key)
		bodies[key] = b
		w.Header().Set("Content-Type", "application/json")
		switch key {
		case "POST /api/v1/imports/analyze":
			_, _ = w.Write([]byte(analysisFixture))
		case "GET /api/v1/databases/shop-postgres":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"database not found"}`))
		case "POST /api/v1/databases", "POST /api/v1/apps":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(b)
		case "POST /api/v1/apps/shop/connections":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"env_var":"DATABASE_URL","database_name":"shop-postgres","field":"url"}`))
		case "PUT /api/v1/apps/shop/git-source":
			_, _ = w.Write([]byte(`{}`))
		case "POST /api/v1/apps/shop/builds":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"att7"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestImportAnalyzePrintsFindingsWithEvidence(t *testing.T) {
	var calls []string
	srv := newAnalyzeServer(t, &calls, map[string][]byte{})
	defer srv.Close()
	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "https://github.com/acme/shop", "--analyze", "--api-url", srv.URL}, &out, &errb, envMap())
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	for _, want := range []string{"Next.js", "next dependency found", "3000", "postgres 16 as shop-postgres", "NEXTAUTH_SECRET", "required, secret", "set by database:shop-postgres"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
	if len(calls) != 1 {
		t.Errorf("calls = %v, want only the analyze call", calls)
	}
}

func TestImportApplyBlockedByMissingRequiredEnv(t *testing.T) {
	var calls []string
	srv := newAnalyzeServer(t, &calls, map[string][]byte{})
	defer srv.Close()
	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "https://github.com/acme/shop", "--apply", "--api-url", srv.URL}, &out, &errb, envMap())
	if code == exitOK || !strings.Contains(errb.String(), "NEXTAUTH_SECRET") {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	for _, c := range calls {
		if c == "POST /api/v1/apps" || c == "POST /api/v1/databases" {
			t.Errorf("nothing may be created, saw %s", c)
		}
	}
}

func TestImportApplyCreatesDatabaseAppConnectionAndBuild(t *testing.T) {
	var calls []string
	bodies := map[string][]byte{}
	srv := newAnalyzeServer(t, &calls, bodies)
	defer srv.Close()
	var out, errb bytes.Buffer
	args := []string{"import", "https://github.com/acme/shop", "--apply", "--env", "NEXTAUTH_SECRET=s3cret", "--api-url", srv.URL, "--json"}
	code := run("cli", args, &out, &errb, envMap())
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	want := []string{
		"POST /api/v1/imports/analyze", "GET /api/v1/databases/shop-postgres", "POST /api/v1/databases", "POST /api/v1/apps",
		"POST /api/v1/apps/shop/connections", "PUT /api/v1/apps/shop/git-source", "POST /api/v1/apps/shop/builds",
	}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v\nwant    %v", calls, want)
	}
	var app apiclient.AppResource
	if err := json.Unmarshal(bodies["POST /api/v1/apps"], &app); err != nil {
		t.Fatal(err)
	}
	if app.Port != 3000 || app.Secrets["NEXTAUTH_SECRET"] != "s3cret" || app.Resources == nil || app.Resources.MemoryBytes != 805306368 {
		t.Errorf("app body = %+v", app)
	}
	if strings.Contains(string(bodies["POST /api/v1/apps/shop/builds"]), "s3cret") {
		t.Error("secret must not reach the build request")
	}
	if !strings.Contains(out.String(), "att7") {
		t.Errorf("output = %s", out.String())
	}
}
