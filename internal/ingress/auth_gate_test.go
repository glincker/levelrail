package ingress

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var updateAuthGateGolden = flag.Bool("update-auth-gate-golden", false, "rewrite testdata/auth_gate_route.golden.json")

func goldenAuthGate() *AuthGate {
	return &AuthGate{
		URL:         "https://auth.internal:9091/api",
		CheckPath:   "/check?rd=1",
		CopyHeaders: []string{"x-user", "X-Email", "X-Name"},
		Timeout:     3 * time.Second,
	}
}

func TestAuthGate_Golden(t *testing.T) {
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "srv",
		ListenAddr: ":443",
		Routes: []ProxyRoute{{
			Hosts: []string{"app.example.com"}, BackendDial: "127.0.0.1:9000",
			WAF:      &WAFConfig{RateLimitRPS: 5},
			AuthGate: goldenAuthGate(),
		}},
	})
	if err != nil {
		t.Fatalf("BuildRoutesConfig() error = %v", err)
	}
	got, err := json.MarshalIndent(cfg.Apps.HTTP.Servers["srv"].Routes, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "auth_gate_route.golden.json")
	if *updateAuthGateGolden {
		if err := os.WriteFile(path, append(got, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path) //nolint:gosec // fixed testdata path
	if err != nil {
		t.Fatalf("read golden (run with -update-auth-gate-golden): %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(got)) {
		t.Fatalf("compiled routes differ from %s; rerun with -update-auth-gate-golden and review the diff\n%s", path, got)
	}
}

func TestAuthGate_ChainOrder(t *testing.T) {
	tests := []struct {
		name   string
		route  ProxyRoute
		expect string
	}{
		{"off by default", ProxyRoute{Hosts: []string{"a.test"}, BackendDial: "x:1"}, "reverse_proxy"},
		{"gate only", ProxyRoute{Hosts: []string{"a.test"}, BackendDial: "x:1", AuthGate: goldenAuthGate()}, "headers reverse_proxy reverse_proxy"},
		{"gate without copy headers", ProxyRoute{Hosts: []string{"a.test"}, BackendDial: "x:1", AuthGate: &AuthGate{URL: "http://auth:80"}}, "reverse_proxy reverse_proxy"},
		{"full chain", ProxyRoute{
			Hosts: []string{"a.test"}, BackendDial: "x:1",
			WAF:       &WAFConfig{Enabled: true, RateLimitRPS: 5},
			BasicAuth: &BasicAuthAccount{Username: "u", Password: "p"},
			AuthGate:  goldenAuthGate(),
			Policy:    goldenPolicy(),
		}, "levelrail_redirects levelrail_geo headers headers subroute headers headers headers subroute headers rate_limit waf authentication headers reverse_proxy levelrail_cache subroute reverse_proxy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := BuildRoutesConfig(RoutesOptions{ServerName: "s", ListenAddr: ":443", Routes: []ProxyRoute{tt.route}})
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Join(handlerNames(t, cfg.Apps.HTTP.Servers["s"].Routes[0].Handle), " ")
			if got != tt.expect {
				t.Fatalf("handlers = %q\nwant      %q", got, tt.expect)
			}
		})
	}
}

func TestAuthGate_StripsClientCopiesBeforeSubrequest(t *testing.T) {
	handlers, err := goldenAuthGate().handlers()
	if err != nil {
		t.Fatal(err)
	}
	strip, ok := handlers[0].(HeadersHandler)
	if !ok || strip.Request == nil {
		t.Fatalf("first gate handler = %#v, want a request headers handler", handlers[0])
	}
	if want := []string{"X-User", "X_User", "X-Email", "X_Email", "X-Name", "X_Name"}; !reflect.DeepEqual(strip.Request.Delete, want) {
		t.Errorf("deleted = %v, want %v", strip.Request.Delete, want)
	}
	proxy, ok := handlers[1].(ReverseProxyHandler)
	if !ok {
		t.Fatalf("second gate handler = %#v, want reverse_proxy", handlers[1])
	}
	if proxy.Rewrite == nil || proxy.Rewrite.Method != "GET" || proxy.Rewrite.URI != "/api/check?rd=1" {
		t.Errorf("rewrite = %+v, want GET /api/check?rd=1", proxy.Rewrite)
	}
	if len(proxy.HandleResponse) != 1 || !reflect.DeepEqual(proxy.HandleResponse[0].Match.StatusCode, []int{2}) {
		t.Fatalf("handle_response = %+v, want exactly one 2xx branch", proxy.HandleResponse)
	}
	if n := len(proxy.HandleResponse[0].Routes); n != 4 {
		t.Errorf("2xx routes = %d, want vars plus one per copy header", n)
	}
}

func TestValidateAuthGateURL(t *testing.T) {
	tests := []struct {
		raw     string
		wantErr bool
	}{
		{"http://auth:9091", false},
		{"https://auth.example.com/base", false},
		{"HTTP://10.0.0.5:4180", false},
		{"http://169.254.169.254", false},
		{"", true},
		{"ftp://auth.example.com", true},
		{"auth.example.com", true},
		{"https://user:pass@auth.example.com", true},
		{"https://user@auth.example.com", true},
		{"https://", true},
		{"http://:9091", true},
		{"https://auth.example.com/?x=1", true},
		{"https://auth.example.com/#frag", true},
		{"https://auth.example.com:99999", true},
		{"https://auth.{env.HOME}.com", true},
		{"javascript:alert(1)", true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			_, err := ValidateAuthGateURL(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAuthGateURL(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeAuthGateCheckPath(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "/", false},
		{"/check", "/check", false},
		{"/check?rd=1", "/check?rd=1", false},
		{"check", "", true},
		{"//evil.example.com/check", "", true},
		{"/a b", "", true},
		{"/{http.request.header.Cookie}", "", true},
		{"/x#frag", "", true},
		{"/" + strings.Repeat("a", 1100), "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := NormalizeAuthGateCheckPath(tt.in)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("NormalizeAuthGateCheckPath(%q) = (%q, %v), want (%q, err=%v)", tt.in, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestNormalizeAuthGateCopyHeaders(t *testing.T) {
	many := make([]string, MaxAuthGateCopyHeaders+1)
	for i := range many {
		many[i] = "X-H" + strings.Repeat("a", i+1)
	}
	tests := []struct {
		name    string
		in      []string
		want    []string
		wantErr bool
	}{
		{"canonicalizes", []string{"x-user", " X-Email "}, []string{"X-User", "X-Email"}, false},
		{"dedupes case insensitively", []string{"X-User", "x-user"}, []string{"X-User"}, false},
		{"empty list", nil, []string{}, false},
		{"empty name", []string{""}, nil, true},
		{"space in name", []string{"X User"}, nil, true},
		{"colon", []string{"X-User:"}, nil, true},
		{"brace placeholder", []string{"{http.vars.x}"}, nil, true},
		{"cookie reserved", []string{"cookie"}, nil, true},
		{"authorization reserved", []string{"Authorization"}, nil, true},
		{"host reserved", []string{"Host"}, nil, true},
		{"exactly the cap", many[:MaxAuthGateCopyHeaders], nil, false},
		{"over the cap", many, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeAuthGateCopyHeaders(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.want != nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthGateTimeoutFromEnv(t *testing.T) {
	tests := []struct {
		raw     string
		want    time.Duration
		wantErr bool
	}{
		{"", defaultAuthGateTimeout, false},
		{"7", 7 * time.Second, false},
		{"1500ms", 1500 * time.Millisecond, false},
		{"0", defaultAuthGateTimeout, true},
		{"soon", defaultAuthGateTimeout, true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := AuthGateTimeoutFromEnv(func(string) string { return tt.raw })
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("got (%v, %v), want (%v, err=%v)", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestAuthGate_InvalidConfigFailsBuild(t *testing.T) {
	_, err := BuildRoutesConfig(RoutesOptions{ServerName: "s", ListenAddr: ":443", Routes: []ProxyRoute{{
		Hosts: []string{"a.test"}, BackendDial: "x:1", AuthGate: &AuthGate{URL: "https://user@auth"},
	}}})
	if err == nil {
		t.Fatal("BuildRoutesConfig() accepted an auth url with userinfo")
	}
}
