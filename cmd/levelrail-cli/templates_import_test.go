package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplatesImport(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
			return
		}
		b := new(bytes.Buffer)
		_, _ = b.ReadFrom(r.Body)
		got = r.Method + " " + r.URL.Path + " " + b.String()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"web-app","name":"web-app","description":"","requires_configuration":false,"created_at":"now"}`))
	}))
	defer srv.Close()
	file := filepath.Join(t.TempDir(), "web-app.yaml")
	if err := os.WriteFile(file, []byte("services:\n  web:\n    image: nginx:1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runCLIExpectOK(t, []string{"templates", "import", "--api-url", srv.URL, file})
	if !strings.HasPrefix(got, "POST /api/v1/templates/custom ") || !strings.Contains(got, `"id":"web-app"`) {
		t.Fatalf("request = %s", got)
	}
	if !strings.Contains(out, "web-app") {
		t.Fatalf("output = %s", out)
	}
}

func TestTemplatesImportOneClick(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := new(bytes.Buffer)
		_, _ = b.ReadFrom(r.Body)
		got = b.String()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"demo","name":"Demo","requires_configuration":false,"created_at":"now"}`))
	}))
	defer srv.Close()
	doc := `{"captainVersion":4,"services":{"web":{"image":"nginx:1","extraKey":{"x":1}}},"vendorOneClickApp":{"displayName":"Demo","description":"d"}}`
	file := filepath.Join(t.TempDir(), "demo.json")
	if err := os.WriteFile(file, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	runCLIExpectOK(t, []string{"templates", "import", "--api-url", srv.URL, "--format", "oneclick", file})
	if !strings.Contains(got, `"name":"Demo"`) || !strings.Contains(got, "nginx:1") {
		t.Fatalf("request = %s", got)
	}
}
