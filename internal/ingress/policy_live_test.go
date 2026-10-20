package ingress

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// TestTrafficPolicy_Live boots the embedded Caddy with every policy kind and
// drives real requests through it, proving the generated JSON loads and
// behaves, not just that it matches a golden file.
func TestTrafficPolicy_Live(t *testing.T) {
	skipUnderLoad(t)
	var assetHits atomic.Int32
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Powered-By", "framework")
		w.Header().Set("X-Seen-Env", r.Header.Get("X-Env"))
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			assetHits.Add(1)
			w.Header().Set("Cache-Control", "public, max-age=60")
		}
		_, _ = fmt.Fprintf(w, "app %s", r.URL.Path) //nolint:gosec // test upstream echoes the path
	}))
	defer app.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seen-Host", r.Host)
		_, _ = fmt.Fprintf(w, "api %s", r.URL.RequestURI()) //nolint:gosec // test upstream echoes the path
	}))
	defer api.Close()
	ext := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seen-Host", r.Host)
		_, _ = fmt.Fprintf(w, "ext %s", r.URL.Path) //nolint:gosec // test upstream echoes the path
	}))
	defer ext.Close()

	SetDefaultGeoResolver(NewGeoResolver(func(k string) string {
		return map[string]string{EnvGeoIPSource: GeoSourceHeader}[k]
	}))
	t.Cleanup(func() { SetDefaultGeoResolver(NewGeoResolver(func(string) string { return "" })) })

	extAddr := netip.MustParseAddrPort(ext.Listener.Addr().String())
	policy := trafficpolicy.Policy{
		Redirects: &trafficpolicy.Redirects{TrailingSlash: trafficpolicy.SlashRemove},
		Geo:       &trafficpolicy.Geo{Mode: trafficpolicy.GeoDeny, Countries: []string{"RU"}, Action: trafficpolicy.GeoActionBlock},
		Headers: &trafficpolicy.Headers{
			Rules: []trafficpolicy.HeaderRule{
				{Side: trafficpolicy.SideRequest, Op: trafficpolicy.OpSet, Name: "X-Env", Value: "prod"},
				{Side: trafficpolicy.SideResponse, Op: trafficpolicy.OpSet, Name: "X-Literal", Value: "{env.HOME}"},
			},
			Security:   &trafficpolicy.SecurityPreset{ContentTypeOptions: true, FrameOptions: "DENY"},
			HideServer: true,
		},
		Cache: &trafficpolicy.Cache{Enabled: true, Rules: []trafficpolicy.CacheRule{{Match: trafficpolicy.Match{Kind: trafficpolicy.MatchPrefix, Path: "/assets"}, TTLSeconds: 30}}},
		Forwarders: &trafficpolicy.Forwarders{Rules: []trafficpolicy.Forwarder{
			{Match: trafficpolicy.Match{Kind: trafficpolicy.MatchPrefix, Path: "/api"}, Action: trafficpolicy.ActionApp, App: "api", StripPrefix: true},
			{Match: trafficpolicy.Match{Kind: trafficpolicy.MatchExact, Path: "/old"}, Action: trafficpolicy.ActionRedirect, URL: "https://example.org/new", RedirectStatus: 301},
			{Match: trafficpolicy.Match{Kind: trafficpolicy.MatchPrefix, Path: "/ext"}, Action: trafficpolicy.ActionURL, URL: "http://ext.example.org/base", StripPrefix: true},
			{Match: trafficpolicy.Match{Kind: trafficpolicy.MatchPrefix, Path: "/blocked"}, Action: trafficpolicy.ActionURL, URL: "http://10.0.0.1"},
		}},
	}
	rp := &RoutePolicy{
		Policy:   policy,
		AppDials: map[string]string{"api": api.Listener.Addr().String()},
		Targets: map[string]trafficpolicy.ResolvedTarget{
			"http://ext.example.org/base": {Scheme: "http", Host: "ext.example.org", Port: fmt.Sprint(extAddr.Port()), Addr: extAddr.Addr(), BasePath: "/base"},
		},
		GeoActive: true,
		Limits:    trafficpolicy.DefaultLimits(),
	}
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	h := DefaultHardening()
	h.TrustedProxies = []string{"127.0.0.1/32"}
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "policy",
		ListenAddr: addr,
		Routes:     []ProxyRoute{{Hosts: []string{"app.test"}, BackendDial: app.Listener.Addr().String(), Policy: rp}},
		HTTPPort:   freePort(t),
		StorageDir: t.TempDir(),
		Hardening:  &h,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Apps.HTTP.Servers["policy"].AutomaticHTTPS = &AutoHTTPSConfig{Disabled: true}
	d := New(testLogger(t))
	if err := d.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("apply: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop(context.Background()) })
	base := "http://" + addr
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(path string, hdr map[string]string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, base+path, nil)
		req.Host = "app.test"
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := noRedirect.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		buf := new(strings.Builder)
		_, _ = fmt.Fprint(buf, readAll(resp))
		return resp, buf.String()
	}

	resp, body := get("/home", nil)
	if body != "app /home" || resp.Header.Get("X-Seen-Env") != "prod" {
		t.Errorf("default route = %d %q seen env %q", resp.StatusCode, body, resp.Header.Get("X-Seen-Env"))
	}
	if resp.Header.Get("X-Powered-By") != "" || resp.Header.Get("Server") != "" {
		t.Errorf("hide server failed: %v", resp.Header)
	}
	if resp.Header.Get("X-Frame-Options") != "DENY" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("security headers missing: %v", resp.Header)
	}
	if resp.Header.Get("X-Literal") != "{env.HOME}" {
		t.Errorf("placeholder expanded in operator header value: %q", resp.Header.Get("X-Literal"))
	}

	if _, body := get("/api/users?x=1", nil); body != "api /users?x=1" {
		t.Errorf("app forwarder = %q", body)
	}
	if _, body := get("/apix", nil); body != "app /apix" {
		t.Errorf("/apix must not match the /api prefix, got %q", body)
	}
	if resp, body := get("/ext/page", nil); body != "ext /base/page" || resp.Header.Get("X-Seen-Host") != fmt.Sprintf("ext.example.org:%d", extAddr.Port()) {
		t.Errorf("url forwarder = %q host %q", body, resp.Header.Get("X-Seen-Host"))
	}
	if resp, _ := get("/blocked", nil); resp.StatusCode != http.StatusBadGateway {
		t.Errorf("unchecked target status = %d, want 502", resp.StatusCode)
	}
	if resp, _ := get("/old", nil); resp.StatusCode != 301 || resp.Header.Get("Location") != "https://example.org/new" {
		t.Errorf("redirect forwarder = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp, _ := get("/docs/?a=1", nil); resp.StatusCode != 308 || resp.Header.Get("Location") != "/docs?a=1" {
		t.Errorf("trailing slash = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	if resp, _ := get("/home", map[string]string{"X-Forwarded-For": "203.0.113.9", "CF-IPCountry": "RU"}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("geo deny = %d, want 403", resp.StatusCode)
	}
	if resp, _ := get("/home", map[string]string{"X-Forwarded-For": "203.0.113.9", "CF-IPCountry": "DE"}); resp.StatusCode != http.StatusOK {
		t.Errorf("geo allow = %d, want 200", resp.StatusCode)
	}

	first, _ := get("/assets/app.css", nil)
	second, body := get("/assets/app.css", nil)
	if first.Header.Get(CacheStatusHeader) != cacheMiss || second.Header.Get(CacheStatusHeader) != cacheHit || body != "app /assets/app.css" || assetHits.Load() != 1 {
		t.Errorf("cache: first %q second %q body %q upstream hits %d", first.Header.Get(CacheStatusHeader), second.Header.Get(CacheStatusHeader), body, assetHits.Load())
	}
	if second.Header.Get("X-Frame-Options") != "DENY" {
		t.Errorf("cache hit lost the security headers")
	}
	if resp, _ := get("/assets/app.css", map[string]string{"Cookie": "s=1"}); resp.Header.Get(CacheStatusHeader) != cacheBypass {
		t.Errorf("cookie request = %q, want BYPASS", resp.Header.Get(CacheStatusHeader))
	}
	DefaultResponseCache().Purge("app.test", PurgeAll, "")
}

func readAll(resp *http.Response) string {
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			return b.String()
		}
	}
}
