package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func appendFile(path, text string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // test temp file
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.WriteString(text)
	return err
}

func kubectlLookup(url string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		switch k {
		case envAPIURL:
			return url, true
		case "VISUAL":
			return "", false
		}
		return "", false
	}
}

func TestKubectlGetWideShowsExtraColumns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/apps" || r.URL.Query().Get("labelSelector") != "env=prod" {
			http.Error(w, "unexpected "+r.URL.String(), http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"kind":"AppList","items":[{"kind":"App","metadata":{"name":"web","namespace":"p1","labels":{"env":"prod"},"resourceVersion":"7","creationTimestamp":"2026-01-01T00:00:00Z"},"spec":{"image":"nginx:1","port":80,"domains":["a.test"]},"status":{"label":"Healthy"}}]}`))
	}))
	defer srv.Close()

	var out, errb bytes.Buffer
	if code := run("lr", []string{"get", "apps", "-l", "env=prod", "-o", "wide", "--token", "t"}, &out, &errb, kubectlLookup(srv.URL)); code != exitOK {
		t.Fatalf("code %d: %s", code, errb.String())
	}
	for _, want := range []string{"IMAGE", "nginx:1", "a.test", "env=prod", "Healthy"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("wide output missing %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	if code := run("lr", []string{"get", "apps", "-l", "env=prod", "-o", "name", "--token", "t"}, &out, &errb, kubectlLookup(srv.URL)); code != exitOK || strings.TrimSpace(out.String()) != "app/web" {
		t.Errorf("name output %q code %d", out.String(), code)
	}
	out.Reset()
	if code := run("lr", []string{"get", "apps", "-l", "env=prod", "--token", "t"}, &out, &errb, kubectlLookup(srv.URL)); code != exitOK || strings.Contains(out.String(), "IMAGE") {
		t.Errorf("table must not carry wide columns: %q", out.String())
	}
}

func TestKubectlRejectsUnknownKind(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run("lr", []string{"get", "pods", "--token", "t"}, &out, &errb, kubectlLookup("http://127.0.0.1:1")); code == exitOK {
		t.Errorf("unknown kind must fail")
	}
}

func TestKubectlPortAndRemoteParsing(t *testing.T) {
	l, r, err := parsePortPair("8080:3000")
	if err != nil || l != 8080 || r != 3000 {
		t.Errorf("pair %d %d %v", l, r, err)
	}
	if l, r, _ = parsePortPair("5433"); l != 5433 || r != 5433 {
		t.Errorf("single %d %d", l, r)
	}
	if _, _, err = parsePortPair("0:99999"); err == nil {
		t.Errorf("out of range port must fail")
	}
	if isDB, name, err := parseForwardTarget("db/main"); err != nil || !isDB || name != "main" {
		t.Errorf("target %v %q %v", isDB, name, err)
	}
	if isDB, name, _ := parseForwardTarget("web"); isDB || name != "web" {
		t.Errorf("bare target %v %q", isDB, name)
	}
	if r, ok := parseRemoteSpec("web/data:conf/app.ini"); !ok || r.App != "web" || r.Volume != "data" || r.Path != "conf/app.ini" {
		t.Errorf("remote %+v %v", r, ok)
	}
	for _, local := range []string{"./a:b", "/tmp/x:y", "plain.txt", "-"} {
		if _, ok := parseRemoteSpec(local); ok {
			t.Errorf("%q must stay local", local)
		}
	}
}

func TestKubectlEditConflictKeepsEdit(t *testing.T) {
	var sentVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v2/apps/web" && r.Method == http.MethodPatch:
			sentVersion = r.URL.Query().Get("resourceVersion")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"resourceVersion precondition failed: web is at 2, not 1"}`))
		case r.URL.Path == "/api/v2/apps/web":
			_, _ = w.Write([]byte(`{"kind":"App","metadata":{"name":"web","resourceVersion":"1"}}`))
		case r.URL.Path == "/api/v1/export":
			_, _ = w.Write([]byte(`{"files":[{"name":"web.yaml","content":"spec:\n  a: 1\n"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := editorRunner
	editorRunner = func(_, path string) error { return appendFile(path, "  b: 2\n") }
	defer func() { editorRunner = old }()

	var out, errb bytes.Buffer
	code := run("lr", []string{"edit", "app", "web", "--yes", "--token", "t"}, &out, &errb, kubectlLookup(srv.URL))
	if code == exitOK || sentVersion != "1" || !strings.Contains(errb.String()+out.String(), "precondition failed") {
		t.Errorf("want conflict, code %d out %q err %q", code, out.String(), errb.String())
	}
}
