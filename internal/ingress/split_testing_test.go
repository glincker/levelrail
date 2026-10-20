package ingress

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/splittest"
)

const (
	testProdHost    = "shop.example.com"
	testVariantHost = "redesign.example.com"
)

func testSplit(wProd, wVar int) ActiveSplit {
	return ActiveSplit{
		TestID: "t1", CookieName: "lr_split", CookieTTL: time.Hour, OverrideParam: "lr_variant",
		Variants: []splittest.Variant{
			{ID: "a1", Label: "Control", Kind: splittest.KindProduction, Weight: wProd},
			{ID: "b2", Label: "Redesign", Kind: splittest.KindBranch, Ref: "redesign", Weight: wVar},
		},
		VariantHosts: map[string]string{"b2": testVariantHost},
	}
}

// splitServer serves both hosts on one listener, like ingress does: the
// production host runs the module, the variant host answers "variant".
func splitServer(t *testing.T) *httptest.Server {
	t.Helper()
	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) error {
		_, _ = io.WriteString(w, "prod")
		return nil
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == testVariantHost {
			_, _ = io.WriteString(w, "variant")
			return
		}
		_ = splitTestingModule{}.ServeHTTP(w, r, next)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, path, cookie string) (string, *http.Response) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = testProdHost
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "lr_split", Value: cookie}) //nolint:gosec // request cookie in a test
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return string(b), res
}

func withSplit(t *testing.T, sp ActiveSplit) {
	t.Helper()
	reg := DefaultSplits()
	reg.SeedForTest(1, 2)
	reg.Set([]string{testProdHost}, sp)
	t.Cleanup(func() { reg.Clear(sp.TestID) })
}

func TestSplitStickyCookieAndOverride(t *testing.T) {
	withSplit(t, testSplit(50, 50))
	srv := splitServer(t)

	tests := []struct {
		name       string
		path       string
		cookie     string
		wantBody   string
		wantCookie string
	}{
		{"cookie to variant sticks", "/", "b2", "variant", ""},
		{"cookie to control sticks", "/", "a1", "prod", ""},
		{"override by label wins over cookie", "/?lr_variant=redesign", "a1", "variant", "b2"},
		{"override by id", "/?lr_variant=a1", "b2", "prod", "a1"},
		{"unknown override ignored", "/?lr_variant=nope", "b2", "variant", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, res := get(t, srv, tc.path, tc.cookie)
			if body != tc.wantBody {
				t.Fatalf("body = %q, want %q", body, tc.wantBody)
			}
			var got string
			for _, c := range res.Cookies() {
				if c.Name == "lr_split" {
					got = c.Value
				}
			}
			if got != tc.wantCookie {
				t.Fatalf("set cookie = %q, want %q", got, tc.wantCookie)
			}
		})
	}
}

func TestSplitFreshVisitorGetsHardenedCookie(t *testing.T) {
	withSplit(t, testSplit(50, 50))
	srv := splitServer(t)
	_, res := get(t, srv, "/", "")
	var c *http.Cookie
	for _, k := range res.Cookies() {
		if k.Name == "lr_split" {
			c = k
		}
	}
	if c == nil {
		t.Fatal("fresh visitor got no split cookie")
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 3600 || c.Path != "/" {
		t.Fatalf("cookie attributes wrong: %+v", c)
	}
	if c.Secure {
		t.Fatal("cookie must not be Secure over plain http")
	}
}

func TestSplitSecureOverForwardedHTTPS(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	if !requestIsHTTPS(r) {
		t.Fatal("forwarded https must count as secure")
	}
	if requestIsHTTPS(httptest.NewRequest(http.MethodGet, "/", nil)) {
		t.Fatal("plain request must not count as secure")
	}
}

func TestSplitInactiveHostSetsNoCookie(t *testing.T) {
	srv := splitServer(t)
	body, res := get(t, srv, "/", "")
	if body != "prod" || len(res.Cookies()) != 0 {
		t.Fatalf("no active split must pass through untouched, got %q with %d cookies", body, len(res.Cookies()))
	}
}

func TestSplitCountsAndDrain(t *testing.T) {
	withSplit(t, testSplit(50, 50))
	srv := splitServer(t)
	for i := 0; i < 3; i++ {
		get(t, srv, "/", "b2")
	}
	get(t, srv, "/", "a1")
	got := DefaultSplits().Drain()
	if got["t1"]["b2"] != 3 || got["t1"]["a1"] != 1 {
		t.Fatalf("drained counts = %v", got)
	}
	if again := DefaultSplits().Drain(); len(again) != 0 {
		t.Fatalf("second drain should be empty, got %v", again)
	}
}

func TestSplitDistributionAcrossFreshVisitors(t *testing.T) {
	withSplit(t, testSplit(70, 30))
	srv := splitServer(t)
	variant := 0
	const n = 600
	for i := 0; i < n; i++ {
		if body, _ := get(t, srv, "/", ""); strings.HasPrefix(body, "variant") {
			variant++
		}
	}
	if share := float64(variant) / n; share < 0.22 || share > 0.38 {
		t.Fatalf("variant share = %.2f, want about 0.30", share)
	}
}

func TestSplitHopHeaderSkipsSplit(t *testing.T) {
	withSplit(t, testSplit(0, 100))
	srv := splitServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Host = testProdHost
	req.Header.Set(splitHopHeader, "1")
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	if string(b) != "prod" {
		t.Fatalf("hop header must bypass the split, got %q", b)
	}
}

func TestSplitsActiveFollowsSetAndClear(t *testing.T) {
	reg := NewSplits()
	if reg.Active([]string{"A.example.com"}) {
		t.Fatal("empty registry must not be active")
	}
	reg.Set([]string{"a.example.com"}, testSplit(50, 50))
	if !reg.Active([]string{"x.example.com", "A.example.com"}) {
		t.Fatal("host match must be case-insensitive")
	}
	reg.Clear("t1")
	if reg.Active([]string{"a.example.com"}) {
		t.Fatal("cleared split must be inactive")
	}
}
