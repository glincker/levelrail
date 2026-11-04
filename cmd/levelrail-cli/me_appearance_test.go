package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRun_MeAppearanceGetSetReset(t *testing.T) {
	var got []string
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":{"theme":"dark"}}`))
	}))
	t.Cleanup(srv.Close)

	stdout, _ := runCLIExpectOK(t, []string{"me", "appearance", "--api-url", srv.URL})
	if !strings.Contains(stdout, `"theme":"dark"`) {
		t.Fatalf("get stdout = %q", stdout)
	}
	f := t.TempDir() + "/a.json"
	if err := writeTestFile(f, `{"theme":"light"}`); err != nil {
		t.Fatal(err)
	}
	runCLIExpectOK(t, []string{"me", "appearance", "set", "--api-url", srv.URL, f})
	if body != `{"theme":"light"}` {
		t.Fatalf("set body = %q", body)
	}
	var out, errb strings.Builder
	if code := run("levelrail-cli-test", []string{"me", "appearance", "reset", "--api-url", srv.URL}, &out, &errb, envMap()); code != exitUsage {
		t.Fatalf("reset without --yes = %d", code)
	}
	runCLIExpectOK(t, []string{"me", "appearance", "reset", "--yes", "--api-url", srv.URL})
	want := []string{"GET /api/v1/me/preferences/appearance", "PUT /api/v1/me/preferences/appearance", "DELETE /api/v1/me/preferences/appearance"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("requests = %v", got)
	}
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
