package ingress

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/staticroutes"
)

func cacheControlFor(t *testing.T, m *staticSiteModule, files map[string]string, target string) string {
	t.Helper()
	for name, body := range files {
		full := filepath.Join(m.Root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	if err := m.ServeHTTP(rec, httptest.NewRequest("GET", target, nil), nil); err != nil {
		t.Fatal(err)
	}
	return rec.Header().Get("Cache-Control")
}

func TestStaticDefaultCacheHashedAssetImmutable(t *testing.T) {
	m := newStaticSiteForTest(t, staticroutes.Input{})
	files := map[string]string{"app.abc123def.js": "x", "assets/index-9f8e7d6c.css": "x", "_astro/page.Bx7k2Q9a.js": "x", "plain-document.pdf": "x"}
	want := "public, max-age=31536000, immutable"
	for _, target := range []string{"/app.abc123def.js", "/assets/index-9f8e7d6c.css", "/_astro/page.Bx7k2Q9a.js"} {
		if got := cacheControlFor(t, m, files, target); got != want {
			t.Fatalf("%s: %q", target, got)
		}
	}
	if got := cacheControlFor(t, m, files, "/plain-document.pdf"); got != "no-cache" {
		t.Fatalf("plain name: %q", got)
	}
}

func TestStaticDefaultCachePagesRevalidate(t *testing.T) {
	m := newStaticSiteForTest(t, staticroutes.Input{})
	files := map[string]string{"manifest.json": "{}", "sw.js": "x", "service-worker.js": "x"}
	for _, target := range []string{"/", "/about.html", "/manifest.json", "/sw.js", "/service-worker.js"} {
		if got := cacheControlFor(t, m, files, target); got != "no-cache" {
			t.Fatalf("%s: %q", target, got)
		}
	}
}

func TestStaticDefaultCacheServiceWorkerIgnoresHash(t *testing.T) {
	if got := defaultCacheControl("/assets/sw.js", 10); got != "no-cache" {
		t.Fatalf("got %q", got)
	}
}

func TestStaticDefaultCacheUserRuleWins(t *testing.T) {
	m := newStaticSiteForTest(t, staticroutes.Input{Headers: "/app.abc123def.js\n  Cache-Control: max-age=60\n/\n  Cache-Control: max-age=5\n"})
	files := map[string]string{"app.abc123def.js": "x"}
	if got := cacheControlFor(t, m, files, "/app.abc123def.js"); got != "max-age=60" {
		t.Fatalf("got %q", got)
	}
	if got := cacheControlFor(t, m, files, "/"); got != "max-age=5" {
		t.Fatalf("got %q", got)
	}
}

func TestStaticDefaultCacheEnvOverride(t *testing.T) {
	t.Setenv(EnvStaticAssetMaxAge, "120")
	m := newStaticSiteForTest(t, staticroutes.Input{})
	got := cacheControlFor(t, m, map[string]string{"app.abc123def.js": "x"}, "/app.abc123def.js")
	if got != "public, max-age=120, immutable" {
		t.Fatalf("got %q", got)
	}
	t.Setenv(EnvStaticCompressMinLength, "1024")
	if n := NewStaticEncodeHandler().MinLength; n != 1024 {
		t.Fatalf("min length %d", n)
	}
}

func TestStaticRouteCompressionOnlyOnStaticRoutes(t *testing.T) {
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "ingress", ListenAddr: ":8443",
		Routes:       []ProxyRoute{{Hosts: []string{"app.example.internal"}, BackendDial: "127.0.0.1:9090"}},
		StaticRoutes: []StaticRoute{{Hosts: []string{"docs.example.internal"}, RootDir: "/data/static/docs"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var generic struct {
		Apps struct {
			HTTP struct {
				Servers map[string]struct {
					Routes []json.RawMessage `json:"routes"`
				} `json:"servers"`
			} `json:"http"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	var static, proxy int
	for _, srv := range generic.Apps.HTTP.Servers {
		for _, r := range srv.Routes {
			s := string(r)
			enc := strings.Count(s, `"handler":"encode"`)
			switch {
			case strings.Contains(s, "docs.example.internal"):
				static += enc
				if !strings.Contains(s, `"zstd"`) || !strings.Contains(s, `"minimum_length":256`) {
					t.Fatalf("encode config: %s", s)
				}
			case strings.Contains(s, "app.example.internal"):
				proxy += enc
			}
		}
	}
	if static != 1 || proxy != 0 {
		t.Fatalf("encode handlers: static=%d proxy=%d", static, proxy)
	}
}
