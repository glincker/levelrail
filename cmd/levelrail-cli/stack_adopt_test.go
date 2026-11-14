package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const adoptBody = "services:\n  web:\n    image: nginx:1.27\n"

func adoptServer(t *testing.T, planned *bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/compose/revisions/3"):
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 3, "compose": adoptBody})
		case strings.HasSuffix(r.URL.Path, "/compose"):
			_ = json.NewEncoder(w).Encode(map[string]any{"app_id": "a", "revision": 3})
		case r.URL.Path == "/api/v1/apply/plan":
			*planned = true
			_ = json.NewEncoder(w).Encode(map[string]any{"plan": map[string]any{"summary": map[string]int{"noop": 1}}})
		default:
			http.NotFound(w, r)
		}
	}))
}

func runAdopt(t *testing.T, srv *httptest.Server, args ...string) (int, string) {
	t.Helper()
	var out, errb bytes.Buffer
	lookup := func(k string) (string, bool) {
		if k == envAPIURL {
			return srv.URL, true
		}
		return "", false
	}
	code := runStack("levelrail-cli", append([]string{"adopt-from-export"}, args...), &out, &errb, lookup)
	return code, out.String() + errb.String()
}

func TestStackAdoptFromExportWritesFileAndRefusesOverwrite(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var planned bool
	srv := adoptServer(t, &planned)
	defer srv.Close()
	dir := t.TempDir()
	doc := "version: 1\nkind: App\nmetadata: {name: shop}\nspec:\n  compose: {path: compose/shop.yaml}\n"
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out := runAdopt(t, srv, "shop", "-f", dir, "--json")
	if code != exitOK || !planned {
		t.Fatalf("code %d planned=%v: %s", code, planned, out)
	}
	var res adoptResult
	if err := json.Unmarshal([]byte(out), &res); err != nil || !res.Noop || len(res.Written) != 1 || len(res.Next) == 0 {
		t.Fatalf("result %+v err %v\n%s", res, err, out)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "compose/shop.yaml"))
	if string(got) != adoptBody {
		t.Fatalf("file: %q", got)
	}
	if code, out := runAdopt(t, srv, "shop", "-f", dir); code == exitOK || !strings.Contains(out, "--force") {
		t.Fatalf("overwrite must be refused: %d %s", code, out)
	}
	if code, _ := runAdopt(t, srv, "shop", "-f", dir, "--force"); code != exitOK {
		t.Fatalf("--force: %d", code)
	}
}
