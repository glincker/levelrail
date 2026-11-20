package ingress

import (
	"net/http/httptest"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func resetTrusted(t *testing.T) {
	t.Helper()
	PublishTrustedProxies(nil)
	t.Cleanup(func() { PublishTrustedProxies(nil) })
}

func TestAbuseClientAddrHonoursSettingOnlyList(t *testing.T) {
	resetTrusted(t)
	t.Setenv(EnvTrustedProxies, "")
	m := &abuseModule{}
	req := httptest.NewRequest("GET", "http://app.test/", nil)
	req.RemoteAddr = "10.0.0.1:4000"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := m.clientAddr(req); got != netip.MustParseAddr("10.0.0.1") {
		t.Fatalf("no trusted list: got %v", got)
	}
	PublishTrustedProxies([]string{"10.0.0.0/8"})
	if got := m.clientAddr(req); got != netip.MustParseAddr("203.0.113.9") {
		t.Fatalf("setting-only list ignored: got %v", got)
	}
	PublishTrustedProxies([]string{"192.168.0.0/16"})
	if got := m.clientAddr(req); got != netip.MustParseAddr("10.0.0.1") {
		t.Fatalf("live change not applied: got %v", got)
	}
}

func TestTrustedProxyPrefixesFallsBackToEnv(t *testing.T) {
	resetTrusted(t)
	t.Setenv(EnvTrustedProxies, "10.0.0.0/8")
	if len(TrustedProxyPrefixes()) != 1 {
		t.Fatal("env seed not used before a list is published")
	}
	PublishTrustedProxies([]string{})
	if len(TrustedProxyPrefixes()) != 0 {
		t.Fatal("published empty list must win over the env seed")
	}
}

func TestGeoResolverHeaderFollowsSettingOnlyList(t *testing.T) {
	resetTrusted(t)
	r := NewGeoResolver(func(string) string { return "" })
	addr := netip.MustParseAddr("1.1.1.1")
	if c, _ := r.Country(addr, "DE", true); c != "" {
		t.Fatalf("header honoured with no trusted list: %q", c)
	}
	if r.Status().Active {
		t.Fatalf("unexpected status %+v", r.Status())
	}
	PublishTrustedProxies([]string{"10.0.0.0/8"})
	if c, src := r.Country(addr, "DE", true); c != "DE" || src != GeoSourceHeader {
		t.Fatalf("setting-only list ignored: %q %q", c, src)
	}
	if !r.Status().Active {
		t.Fatal("status not live")
	}
	PublishTrustedProxies([]string{})
	if c, _ := r.Country(addr, "DE", true); c != "" {
		t.Fatalf("live clear not applied: %q", c)
	}
}

// Redirects trust Caddy's trusted-proxy var, built from the resolved list, so
// this guard keeps every other reader on the shared accessor.
func TestNoDirectTrustedProxyEnvReads(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Skip("not a git checkout")
	}
	dir := strings.TrimSpace(string(root))
	allowed := map[string]bool{
		"internal/ingress/hardening.go":           true,
		"internal/ingress/trusted_resolved.go":    true,
		"internal/ingress/edge_overrides.go":      true,
		"internal/ingress/policy_geo_resolver.go": true,
		"internal/api/trusted_proxies.go":         true,
		"internal/api/device_url.go":              true,
		"internal/api/ingress_edge.go":            true,
	}
	read := regexp.MustCompile(`(Getenv|LookupEnv|getenv)\(\s*(ingress\.|ingressdriver\.)?(EnvTrustedProxies|envTrustedProxies|"APP_INGRESS_TRUSTED_PROXIES")`)
	for _, sub := range []string{"internal", "cmd"} {
		_ = filepath.WalkDir(filepath.Join(dir, sub), func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(dir, p)
			b, _ := os.ReadFile(p) //nolint:gosec // walking our own source tree
			if read.Match(b) && !allowed[rel] {
				t.Errorf("%s reads the trusted proxy env directly; use TrustedProxyPrefixes", rel)
			}
			return nil
		})
	}
}
