package standalone

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		in, kind, canonical, wantErr string
	}{
		{in: "3000", kind: KindPort, canonical: "127.0.0.1:3000"},
		{in: ":3000", kind: KindPort, canonical: "127.0.0.1:3000"},
		{in: "127.0.0.1:3000", kind: KindPort, canonical: "127.0.0.1:3000"},
		{in: "localhost:8080", kind: KindPort, canonical: "127.0.0.1:8080"},
		{in: "10.0.0.5:8080", kind: KindAddress, canonical: "10.0.0.5:8080"},
		{in: "[fd12::5]:80", kind: KindAddress, canonical: "[fd12::5]:80"},
		{in: "[::1]:9000", kind: KindAddress, canonical: "[::1]:9000"},
		{in: "db.internal:5000", kind: KindAddress, canonical: "db.internal:5000"},
		{in: "https://Example.org:8443/base/", kind: KindURL, canonical: "https://example.org:8443/base"},
		{in: "http://example.org", kind: KindURL, canonical: "http://example.org"},
		{in: "app:web", kind: KindApp, canonical: "app:web"},
		{in: "app:web:9090", kind: KindApp, canonical: "app:web:9090"},
		{in: "container:redis-ui:8081", kind: KindContainer, canonical: "container:redis-ui:8081"},
		{in: "", wantErr: "required"},
		{in: "0", wantErr: "out of range"},
		{in: "65536", wantErr: "out of range"},
		{in: "127.0.0.1:0", wantErr: "out of range"},
		{in: "host:-1", wantErr: "number"},
		{in: "https://user:pw@example.org", wantErr: "credentials"}, //nolint:gosec // rejected-credentials fixture, not a secret
		{in: "https://example.org/?a=1", wantErr: "query"},
		{in: "https://example.org/#x", wantErr: "query"},
		{in: "ftp://example.org", wantErr: "http and https"},
		{in: "https://example.org:99999", wantErr: "out of range"},
		{in: "bad host:80", wantErr: "spaces"},
		{in: "{upstream}:80", wantErr: "braces"},
		{in: "-bad.example:80", wantErr: "valid hostname"},
		{in: "[fe80::1%eth0]:80", wantErr: "zoned"},
		{in: "container:redis", wantErr: "container:NAME:PORT"},
		{in: "app:", wantErr: "app name"},
		{in: "justtext", wantErr: "use a port"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseTarget(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseTarget(%q) error = %v, want containing %q", tt.in, err, tt.wantErr)
				}
				if !errors.Is(err, ErrInvalidTarget) {
					t.Fatalf("error %v does not wrap ErrInvalidTarget", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTarget(%q): %v", tt.in, err)
			}
			if got.Kind != tt.kind || got.String() != tt.canonical {
				t.Fatalf("ParseTarget(%q) = %s %q, want %s %q", tt.in, got.Kind, got.String(), tt.kind, tt.canonical)
			}
			again, err := ParseTarget(got.String())
			if err != nil || again.String() != got.String() {
				t.Fatalf("canonical form %q does not round trip: %v %q", got.String(), err, again.String())
			}
		})
	}
}

type fakeResolver map[string][]netip.Addr

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := f[host]; ok {
		return a, nil
	}
	return nil, errors.New("no such host")
}

func TestAssessRejectsOutOfRangePort(t *testing.T) {
	for _, port := range []int{0, -1, 65536, 1<<16 + 3000} {
		_, err := Assess(context.Background(), Target{Kind: KindPort, Host: "127.0.0.1", Port: port}, Env{})
		if !errors.Is(err, ErrInvalidTarget) {
			t.Errorf("port %d: err = %v, want ErrInvalidTarget", port, err)
		}
	}
}

func TestAssess(t *testing.T) {
	env := Env{
		Resolver: fakeResolver{
			"metadata.example": {netip.MustParseAddr("169.254.169.254")},
			"mixed.example":    {netip.MustParseAddr("203.0.113.9"), netip.MustParseAddr("169.254.1.1")},
			"self.example":     {netip.MustParseAddr("198.51.100.7")},
			"lan.example":      {netip.MustParseAddr("192.168.1.20")},
			"public.example":   {netip.MustParseAddr("203.0.113.9")},
		},
		ControlPorts: []ControlPort{{Port: 2019, Label: "ingress admin API", AlwaysBlock: true}, {Port: 8080, Label: "dashboard and API"}},
		OwnAddresses: []string{"198.51.100.7"},
		IngressPorts: []int{8443},
	}
	tests := []struct {
		name     string
		target   string
		wantErr  error
		internal bool
		confirms int
		control  bool
	}{
		{name: "loopback port", target: "3000", internal: true, confirms: 1},
		{name: "private ip", target: "10.0.0.5:8080", internal: true, confirms: 1},
		{name: "private via dns", target: "lan.example:80", internal: true, confirms: 1},
		{name: "public url", target: "https://public.example", confirms: 0},
		{name: "ipv6 loopback", target: "[::1]:3000", internal: true, confirms: 1},
		{name: "ipv6 ula", target: "[fd12::5]:80", internal: true, confirms: 1},
		{name: "metadata ip", target: "169.254.169.254:80", wantErr: ErrBlocked},
		{name: "aws v6 metadata", target: "[fd00:ec2::254]:80", wantErr: ErrBlocked},
		{name: "link local v6", target: "[fe80::1]:80", wantErr: ErrBlocked},
		{name: "metadata via dns", target: "http://metadata.example", wantErr: ErrBlocked},
		{name: "one bad record poisons all", target: "mixed.example:80", wantErr: ErrBlocked},
		{name: "unspecified", target: "0.0.0.0:80", wantErr: ErrBlocked},
		{name: "admin api always blocked", target: "2019", wantErr: ErrBlocked},
		{name: "dashboard port needs confirm", target: "8080", internal: true, confirms: 2, control: true},
		{name: "loop to own public ip", target: "self.example:443", wantErr: ErrLoop},
		{name: "loop to own ingress port", target: "127.0.0.1:8443", wantErr: ErrLoop},
		{name: "loop to local 80", target: "80", wantErr: ErrLoop},
		{name: "own public ip other port ok", target: "198.51.100.7:3000", confirms: 0},
		{name: "app needs nothing", target: "app:web", confirms: 0},
		{name: "container confirms", target: "container:ui:8081", internal: true, confirms: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := ParseTarget(tt.target)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			a, err := Assess(context.Background(), target, env)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Assess error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Assess: %v", err)
			}
			if a.Internal != tt.internal || len(a.NeedsConfirmation) != tt.confirms || (a.ControlPort != nil) != tt.control {
				t.Fatalf("Assess = %+v, want internal=%v confirms=%d control=%v", a, tt.internal, tt.confirms, tt.control)
			}
		})
	}
}

func TestOptionsValidate(t *testing.T) {
	port, _ := ParseTarget("3000")
	secure, _ := ParseTarget("https://example.org")
	tests := []struct {
		name    string
		target  Target
		opts    Options
		wantErr bool
		check   func(Options) bool
	}{
		{name: "defaults", target: port, opts: Options{WebSocket: true}, check: func(o Options) bool { return o.EffectiveHostMode(port) == HostPreserve }},
		{name: "url forces tls", target: secure, opts: Options{}, check: func(o Options) bool { return o.UpstreamTLS && o.EffectiveHostMode(secure) == HostUpstream }},
		{name: "skip verify needs tls", target: port, opts: Options{TLSSkipVerify: true}, wantErr: true},
		{name: "skip verify with tls", target: port, opts: Options{UpstreamTLS: true, TLSSkipVerify: true}},
		{name: "custom host", target: port, opts: Options{HostHeader: "Internal.Example"}, check: func(o Options) bool { return o.HostHeader == "internal.example" }},
		{name: "bad host", target: port, opts: Options{HostHeader: "{bad}"}, wantErr: true},
		{name: "strip prefix normalized", target: port, opts: Options{StripPrefix: "app/"}, check: func(o Options) bool { return o.StripPrefix == "/app" }},
		{name: "strip root rejected", target: port, opts: Options{StripPrefix: "/"}, wantErr: true},
		{name: "strip placeholder rejected", target: port, opts: Options{StripPrefix: "/{x}"}, wantErr: true},
		{name: "timeout range", target: port, opts: Options{ReadTimeoutSeconds: MaxReadTimeoutSeconds + 1}, wantErr: true},
		{name: "dial range", target: port, opts: Options{DialTimeoutSeconds: -1}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.opts.Validate(tt.target)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && tt.check != nil && !tt.check(got) {
				t.Fatalf("Validate = %+v failed check", got)
			}
		})
	}
}
