package ingress

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// writeTestMMDB writes a minimal IPv4 country database mapping network to
// iso, following the MaxMind DB format spec (24 bit records).
func writeTestMMDB(t *testing.T, network netip.Prefix, iso string) string {
	t.Helper()
	bits := network.Bits()
	nodeCount := bits
	addr := network.Addr().As4()
	var tree []byte
	put24 := func(v int) { tree = append(tree, byte(v>>16), byte(v>>8), byte(v)) } //nolint:gosec // 24 bit records by design
	for i := 0; i < bits; i++ {
		bit := (addr[i/8] >> (7 - uint(i%8))) & 1
		next := i + 1
		if i == bits-1 {
			next = nodeCount + 16
		}
		if bit == 0 {
			put24(next)
			put24(nodeCount)
		} else {
			put24(nodeCount)
			put24(next)
		}
	}
	str := func(s string) []byte { return append([]byte{0x40 | byte(len(s))}, s...) } //nolint:gosec // short fixed strings
	var data []byte
	data = append(data, 0xE1)
	data = append(data, str("country")...)
	data = append(data, 0xE1)
	data = append(data, str("iso_code")...)
	data = append(data, str(iso)...)

	var meta []byte
	meta = append(meta, 0xE9)
	meta = append(meta, str("binary_format_major_version")...)
	meta = append(meta, 0xA1, 0x02)
	meta = append(meta, str("binary_format_minor_version")...)
	meta = append(meta, 0xA0)
	meta = append(meta, str("build_epoch")...)
	meta = append(meta, 0x01, 0x02, 0x01)
	meta = append(meta, str("database_type")...)
	meta = append(meta, str("Test-Country")...)
	meta = append(meta, str("description")...)
	meta = append(meta, 0xE0)
	meta = append(meta, str("ip_version")...)
	meta = append(meta, 0xA1, 0x04)
	meta = append(meta, str("languages")...)
	meta = append(meta, 0x00, 0x04)
	meta = append(meta, str("node_count")...)
	meta = append(meta, 0xC1, byte(nodeCount)) //nolint:gosec // at most 32 nodes
	meta = append(meta, str("record_size")...)
	meta = append(meta, 0xA1, 0x18)

	var file []byte
	file = append(file, tree...)
	file = append(file, make([]byte, 16)...)
	file = append(file, data...)
	file = append(file, "\xAB\xCD\xEFMaxMind.com"...)
	file = append(file, meta...)
	path := filepath.Join(t.TempDir(), "country.mmdb")
	if err := os.WriteFile(path, file, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGeoResolverSources(t *testing.T) {
	db := writeTestMMDB(t, netip.MustParsePrefix("81.2.69.0/24"), "GB")
	tests := []struct {
		name        string
		env         map[string]string
		addr        string
		header      string
		trusted     bool
		wantCountry string
		wantSource  string
		active      bool
	}{
		{"mmdb lookup", map[string]string{EnvGeoIPDB: db}, "81.2.69.160", "", false, "GB", GeoSourceMMDB, true},
		{"mmdb miss", map[string]string{EnvGeoIPDB: db}, "8.8.8.8", "", false, "", GeoSourceNone, true},
		{"header ignored from untrusted peer", map[string]string{EnvGeoIPDB: db, EnvTrustedProxies: "10.0.0.0/8"}, "81.2.69.1", "RU", false, "GB", GeoSourceMMDB, true},
		{"header honoured from trusted peer", map[string]string{EnvGeoIPDB: db, EnvTrustedProxies: "10.0.0.0/8"}, "81.2.69.1", "ru", true, "RU", GeoSourceHeader, true},
		{"auto without trusted proxies skips header", map[string]string{}, "81.2.69.1", "RU", true, "", GeoSourceNone, false},
		{"explicit header source", map[string]string{EnvGeoIPSource: "header", EnvGeoIPCountryHeader: "X-Country"}, "1.1.1.1", "DE", true, "DE", GeoSourceHeader, true},
		{"garbage header falls through", map[string]string{EnvGeoIPSource: "header"}, "1.1.1.1", "XX", true, "", GeoSourceNone, true},
		{"none disables", map[string]string{EnvGeoIPSource: "none", EnvGeoIPDB: db}, "81.2.69.1", "", false, "", GeoSourceNone, false},
		{"broken db reported", map[string]string{EnvGeoIPDB: filepath.Join(t.TempDir(), "missing.mmdb")}, "81.2.69.1", "", false, "", GeoSourceNone, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewGeoResolver(func(k string) string { return tt.env[k] })
			c, src := r.Country(netip.MustParseAddr(tt.addr), tt.header, tt.trusted)
			if c != tt.wantCountry || src != tt.wantSource {
				t.Fatalf("Country() = %q, %q, want %q, %q", c, src, tt.wantCountry, tt.wantSource)
			}
			if r.Status().Active != tt.active {
				t.Fatalf("Status() = %+v, want active %v", r.Status(), tt.active)
			}
		})
	}
	broken := NewGeoResolver(func(k string) string { return map[string]string{EnvGeoIPDB: "/nonexistent.mmdb"}[k] })
	if broken.Status().Error == "" {
		t.Error("missing database not reported in status")
	}
}

func geoRequest(remote, clientIP string, trusted bool, hdr map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "http://app.test/", nil)
	r.RemoteAddr = remote
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	vars := map[string]any{caddyhttp.TrustedProxyVarKey: trusted}
	if clientIP != "" {
		vars[caddyhttp.ClientIPVarKey] = clientIP
	}
	return r.WithContext(context.WithValue(r.Context(), caddyhttp.VarsCtxKey, vars))
}

func TestGeoModule(t *testing.T) {
	SetDefaultGeoResolver(NewGeoResolver(func(k string) string {
		return map[string]string{EnvGeoIPSource: GeoSourceHeader}[k]
	}))
	t.Cleanup(func() { SetDefaultGeoResolver(NewGeoResolver(func(string) string { return "" })) })
	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})
	deny := trafficpolicy.Geo{Mode: trafficpolicy.GeoDeny, Countries: []string{"RU"}, Action: trafficpolicy.GeoActionBlock, Exempt: []string{"203.0.113.50"}}
	redirect := deny
	redirect.Action, redirect.RedirectURL = trafficpolicy.GeoActionRedirect, "https://example.com/blocked"
	page := deny
	page.Action, page.Body, page.StatusCode = trafficpolicy.GeoActionErrorPage, "<h1>no</h1>", 451
	tests := []struct {
		name   string
		rule   trafficpolicy.Geo
		req    *http.Request
		status int
	}{
		{"trusted proxy header blocks", deny, geoRequest("127.0.0.1:1", "203.0.113.9", true, map[string]string{"CF-IPCountry": "RU"}), 403},
		{"spoofed header from untrusted peer ignored", deny, geoRequest("203.0.113.9:1", "203.0.113.9", false, map[string]string{"CF-IPCountry": "RU"}), 200},
		{"private client never blocked", deny, geoRequest("127.0.0.1:1", "10.0.0.8", true, map[string]string{"CF-IPCountry": "RU"}), 200},
		{"exempt client", deny, geoRequest("127.0.0.1:1", "203.0.113.50", true, map[string]string{"CF-IPCountry": "RU"}), 200},
		{"other country passes", deny, geoRequest("127.0.0.1:1", "203.0.113.9", true, map[string]string{"CF-IPCountry": "DE"}), 200},
		{"redirect action", redirect, geoRequest("127.0.0.1:1", "203.0.113.9", true, map[string]string{"CF-IPCountry": "RU"}), 302},
		{"error page action", page, geoRequest("127.0.0.1:1", "203.0.113.9", true, map[string]string{"CF-IPCountry": "RU"}), 451},
		{"no client var uses remote addr", deny, geoRequest("203.0.113.9:1", "", true, map[string]string{"CF-IPCountry": "RU"}), 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &geoModule{Rule: tt.rule}
			if err := m.Provision(caddyCtx()); err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			if err := m.ServeHTTP(rec, tt.req, next); err != nil {
				t.Fatal(err)
			}
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
		})
	}
	bad := &geoModule{Rule: trafficpolicy.Geo{Exempt: []string{"nope"}}}
	if err := bad.Provision(caddyCtx()); err == nil {
		t.Error("Provision() accepted an invalid exempt entry")
	}
}
