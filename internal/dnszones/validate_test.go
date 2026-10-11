package dnszones

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func w(v int64) *int64 { return &v }

func TestNormalize(t *testing.T) {
	const zone = "example.com"
	tests := []struct {
		name       string
		in         RecordSet
		wantErr    string
		wantName   string
		wantValues []string
		wantTTL    int
	}{
		{name: "A apex default ttl", in: RecordSet{Name: "", Type: "a", Values: []string{"203.0.113.1", "203.0.113.1"}}, wantName: Apex, wantValues: []string{"203.0.113.1"}, wantTTL: 300},
		{name: "A rejects ipv6", in: RecordSet{Name: "www", Type: "A", Values: []string{"2001:db8::1"}}, wantErr: "not valid A"},
		{name: "AAAA ok", in: RecordSet{Name: "www.example.com.", Type: "AAAA", Values: []string{"2001:DB8::1"}}, wantName: "www", wantValues: []string{"2001:db8::1"}},
		{name: "CNAME strips dot", in: RecordSet{Name: "blog", Type: "CNAME", Values: []string{"Host.Example.NET."}}, wantValues: []string{"host.example.net"}},
		{name: "CNAME at apex symbol", in: RecordSet{Name: "www", Type: "CNAME", Values: []string{"@"}}, wantValues: []string{"example.com"}},
		{name: "CNAME single value", in: RecordSet{Name: "x", Type: "CNAME", Values: []string{"a.net", "b.net"}}, wantErr: "exactly one value"},
		{name: "MX", in: RecordSet{Name: "@", Type: "MX", Values: []string{"10 mail.example.com"}}, wantValues: []string{"10 mail.example.com"}},
		{name: "MX missing priority", in: RecordSet{Name: "@", Type: "MX", Values: []string{"mail.example.com"}}, wantErr: "not valid MX"},
		{name: "TXT keeps raw text", in: RecordSet{Name: "@", Type: "TXT", Values: []string{`v=spf1 include:"x" ~all`}}, wantValues: []string{`v=spf1 include:"x" ~all`}},
		{name: "TXT long", in: RecordSet{Name: "@", Type: "TXT", Values: []string{strings.Repeat("a", 300)}}, wantValues: []string{strings.Repeat("a", 300)}},
		{name: "CAA", in: RecordSet{Name: "@", Type: "CAA", Values: []string{`0 issue "letsencrypt.org"`}}, wantValues: []string{`0 issue "letsencrypt.org"`}},
		{name: "CAA bad tag", in: RecordSet{Name: "@", Type: "CAA", Values: []string{`0 foo "x"`}}, wantErr: "CAA tag"},
		{name: "SRV", in: RecordSet{Name: "_sip._tls", Type: "SRV", Values: []string{"100 1 443 sip.example.net."}}, wantValues: []string{"100 1 443 sip.example.net"}},
		{name: "SRV short", in: RecordSet{Name: "_sip._tls", Type: "SRV", Values: []string{"1 443 x"}}, wantErr: "not valid SRV"},
		{name: "NS subdomain", in: RecordSet{Name: "dev", Type: "NS", Values: []string{"ns1.other.net"}}, wantValues: []string{"ns1.other.net"}},
		{name: "NS apex rejected", in: RecordSet{Name: "@", Type: "NS", Values: []string{"ns1.other.net"}}, wantErr: "apex NS"},
		{name: "unsupported type", in: RecordSet{Name: "x", Type: "PTR", Values: []string{"a"}}, wantErr: "type must be"},
		{name: "ttl too low", in: RecordSet{Name: "x", Type: "A", TTL: 5, Values: []string{"1.2.3.4"}}, wantErr: "ttl must be"},
		{name: "ttl auto", in: RecordSet{Name: "x", Type: "A", TTL: 1, Values: []string{"1.2.3.4"}}, wantTTL: 1},
		{name: "wildcard name", in: RecordSet{Name: "*.apps", Type: "A", Values: []string{"1.2.3.4"}}, wantName: "*.apps"},
		{name: "nested wildcard", in: RecordSet{Name: "a.*", Type: "A", Values: []string{"1.2.3.4"}}, wantErr: "leftmost"},
		{name: "proxied TXT", in: RecordSet{Name: "x", Type: "TXT", Proxied: true, Values: []string{"a"}}, wantErr: "proxied"},
		{name: "empty values", in: RecordSet{Name: "x", Type: "A"}, wantErr: "at least one value"},
		{name: "weighted needs weight", in: RecordSet{Name: "x", Type: "A", Routing: RoutingWeighted, SetIdentifier: "a", Values: []string{"1.2.3.4"}}, wantErr: "weight between"},
		{name: "weighted ok", in: RecordSet{Name: "x", Type: "A", Routing: RoutingWeighted, SetIdentifier: "a", Weight: w(10), Values: []string{"1.2.3.4"}}},
		{name: "failover needs role", in: RecordSet{Name: "x", Type: "A", Routing: RoutingFailover, SetIdentifier: "a", Values: []string{"1.2.3.4"}}, wantErr: "PRIMARY"},
		{name: "routed needs identifier", in: RecordSet{Name: "x", Type: "A", Routing: RoutingMultivalue, Values: []string{"1.2.3.4"}}, wantErr: "set_identifier is required"},
		{name: "simple with identifier", in: RecordSet{Name: "x", Type: "A", SetIdentifier: "a", Values: []string{"1.2.3.4"}}, wantErr: "apply only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize(tt.in, zone, 300)
			if tt.wantErr != "" {
				var ve *ValidationError
				if err == nil || !errors.As(err, &ve) || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if tt.wantName != "" && got.Name != tt.wantName {
				t.Errorf("name = %q, want %q", got.Name, tt.wantName)
			}
			if tt.wantValues != nil && !slices.Equal(got.Values, tt.wantValues) {
				t.Errorf("values = %q, want %q", got.Values, tt.wantValues)
			}
			if tt.wantTTL != 0 && got.TTL != tt.wantTTL {
				t.Errorf("ttl = %d, want %d", got.TTL, tt.wantTTL)
			}
		})
	}
}

func TestConflicts(t *testing.T) {
	existing := []RecordSet{
		{Name: Apex, Type: "NS", Values: []string{"ns1.x"}},
		{Name: Apex, Type: "SOA"},
		{Name: "www", Type: "A", Values: []string{"1.2.3.4"}},
		{Name: "blog", Type: "CNAME", Values: []string{"x.net"}},
		{Name: "lb", Type: "A", SetIdentifier: "a", Routing: RoutingWeighted},
	}
	cf := Capabilities{Proxied: true, ApexCNAME: true}
	r53 := Capabilities{Routing: true}
	tests := []struct {
		name   string
		rs     RecordSet
		caps   Capabilities
		ignore *Key
		want   []string
	}{
		{name: "cname over A", rs: RecordSet{Name: "www", Type: "CNAME"}, caps: r53, want: []string{IssueCNAMEExclusive}},
		{name: "A over cname", rs: RecordSet{Name: "blog", Type: "TXT"}, caps: r53, want: []string{IssueCNAMEExclusive}},
		{name: "apex cname route53", rs: RecordSet{Name: Apex, Type: "CNAME"}, caps: r53, want: []string{IssueApexCNAME}},
		{name: "apex cname cloudflare warns", rs: RecordSet{Name: Apex, Type: "CNAME"}, caps: cf, want: []string{IssueApexFlattened}},
		{name: "duplicate", rs: RecordSet{Name: "www", Type: "A"}, caps: r53, want: []string{IssueExists}},
		{name: "update ignores self", rs: RecordSet{Name: "www", Type: "A"}, caps: r53, ignore: &Key{Name: "www", Type: "A"}},
		{name: "mixed routing", rs: RecordSet{Name: "lb", Type: "A"}, caps: r53, want: []string{IssueMixedRouting}},
		{name: "second weighted ok", rs: RecordSet{Name: "lb", Type: "A", SetIdentifier: "b"}, caps: r53},
		{name: "no conflict", rs: RecordSet{Name: "api", Type: "A"}, caps: r53},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var codes []string
			for _, is := range Conflicts(existing, tt.rs, tt.caps, tt.ignore) {
				codes = append(codes, is.Code)
			}
			if !slices.Equal(codes, tt.want) {
				t.Errorf("codes = %v, want %v", codes, tt.want)
			}
		})
	}
}

func TestCheckCapabilities(t *testing.T) {
	cf := (&Cloudflare{}).Capabilities()
	r53 := (&Route53{}).Capabilities()
	tests := []struct {
		name string
		rs   RecordSet
		caps Capabilities
		ok   bool
	}{
		{"proxied on cloudflare", RecordSet{Proxied: true}, cf, true},
		{"proxied on route53", RecordSet{Proxied: true}, r53, false},
		{"ttl auto on route53", RecordSet{TTL: TTLAuto}, r53, false},
		{"weighted on cloudflare", RecordSet{Routing: RoutingWeighted}, cf, false},
		{"weighted on route53", RecordSet{Routing: RoutingWeighted}, r53, true},
		{"alias", RecordSet{Alias: &Alias{}}, r53, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckCapabilities(tt.rs, tt.caps)
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, ok want %v", err, tt.ok)
			}
			if err != nil && !errors.Is(err, ErrUnsupported) {
				t.Errorf("err %v is not ErrUnsupported", err)
			}
		})
	}
}

func TestNames(t *testing.T) {
	tests := []struct{ in, zone, rel, fqdn string }{
		{"", "example.com", Apex, "example.com"},
		{"@", "example.com.", Apex, "example.com"},
		{"WWW.Example.com.", "example.com", "www", "www.example.com"},
		{"*", "example.com", "*", "*.example.com"},
		{"a.b", "example.com", "a.b", "a.b.example.com"},
	}
	for _, tt := range tests {
		if got := RelativeName(tt.in, tt.zone); got != tt.rel {
			t.Errorf("RelativeName(%q) = %q, want %q", tt.in, got, tt.rel)
		}
		if got := FQDN(tt.in, tt.zone); got != tt.fqdn {
			t.Errorf("FQDN(%q) = %q, want %q", tt.in, got, tt.fqdn)
		}
	}
	if got := unescapeOctal(`\052.example.com.`); got != "*.example.com." {
		t.Errorf("unescapeOctal = %q", got)
	}
	long := strings.Repeat("b", 600)
	if got := unquoteTXT(quoteTXT(long)); got != long {
		t.Errorf("txt round trip lost data: %d bytes", len(got))
	}
	if got := unquoteTXT(`"a \"q\"" "b"`); got != `a "q"b` {
		t.Errorf("unquoteTXT = %q", got)
	}
}

func TestValidateHealthCheck(t *testing.T) {
	tests := []struct {
		hc HealthCheck
		ok bool
	}{
		{HealthCheck{Type: "HTTPS", FQDN: "app.example.com"}, true},
		{HealthCheck{Type: "TCP", IPAddress: "1.2.3.4", Port: 5432}, true},
		{HealthCheck{Type: "TCP", IPAddress: "1.2.3.4"}, false},
		{HealthCheck{Type: "ICMP", IPAddress: "1.2.3.4"}, false},
		{HealthCheck{Type: "HTTP"}, false},
	}
	for _, tt := range tests {
		if err := ValidateHealthCheck(tt.hc); (err == nil) != tt.ok {
			t.Errorf("%+v: err = %v", tt.hc, err)
		}
	}
}
