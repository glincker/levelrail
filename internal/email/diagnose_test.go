package email

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeDNS struct {
	txt   map[string][]string
	cname map[string]string
	host  map[string][]string
}

func (f fakeDNS) LookupTXT(_ context.Context, n string) ([]string, error) {
	if v, ok := f.txt[n]; ok {
		return v, nil
	}
	return nil, errors.New("nxdomain")
}

func (f fakeDNS) LookupCNAME(_ context.Context, n string) (string, error) {
	if v, ok := f.cname[n]; ok {
		return v, nil
	}
	return n + ".", nil
}

func (f fakeDNS) LookupHost(_ context.Context, n string) ([]string, error) {
	if v, ok := f.host[n]; ok {
		return v, nil
	}
	return nil, errors.New("nxdomain")
}

// txtOnlyDNS hides LookupHost so the proxy check must stay silent.
type txtOnlyDNS struct{ f fakeDNS }

func (t txtOnlyDNS) LookupTXT(ctx context.Context, n string) ([]string, error) {
	return t.f.LookupTXT(ctx, n)
}

func (t txtOnlyDNS) LookupCNAME(ctx context.Context, n string) (string, error) {
	return t.f.LookupCNAME(ctx, n)
}

func TestDiagnoseSPFLookupCount(t *testing.T) {
	dns := fakeDNS{txt: map[string][]string{
		"a.test": {"v=spf1 include:b.test mx ~all"},
		"b.test": {"v=spf1 a include:c.test -all"},
		"c.test": {"v=spf1 exists:x.test redirect=a.test"},
		"loop":   {"v=spf1 include:loop ~all"},
	}}
	cases := []struct {
		name, record string
		depth, want  int
	}{
		{"flat", "v=spf1 ip4:1.2.3.4 ~all", 10, 0},
		{"mechanisms", "v=spf1 a mx ptr exists:x.test ~all", 10, 4},
		{"qualifier and cidr", "v=spf1 -a/24 +mx:mail.test/24 ~all", 10, 2},
		{"nested", dns.txt["a.test"][0], 10, 8},
		{"depth cut", dns.txt["a.test"][0], 1, 4},
		{"loop ends", "v=spf1 include:loop ~all", 10, 2},
		{"missing include", "v=spf1 include:gone.test ~all", 10, 1},
	}
	for _, c := range cases {
		if got := CountSPFLookups(context.Background(), dns, c.record, c.depth); got != c.want {
			t.Errorf("%s: %d lookups, want %d", c.name, got, c.want)
		}
	}
}

func TestDiagnoseSPFTooManyAndMultiple(t *testing.T) {
	cases := []struct {
		name string
		txt  []string
		code string
	}{
		{"duplicate records", []string{"v=spf1 mx ~all", "v=spf1 a ~all"}, "spf_multiple"},
		{"too many lookups", []string{"v=spf1 a mx a mx a mx a mx a mx a ~all"}, CodeSPFTooManyLookups},
		{"ten is fine", []string{"v=spf1 a mx a mx a mx a mx a mx ~all"}, "spf_ok"},
		{"missing", nil, "spf_missing"},
	}
	for _, c := range cases {
		dns := fakeDNS{txt: map[string][]string{"example.com": c.txt}}
		if got := CheckSPF(context.Background(), dns, "example.com", "").Code; got != c.code {
			t.Errorf("%s: code %s, want %s", c.name, got, c.code)
		}
	}
}

func TestDiagnoseSPFLimitFromEnv(t *testing.T) {
	t.Setenv("APP_SPF_LOOKUP_LIMIT", "2")
	dns := fakeDNS{txt: map[string][]string{"example.com": {"v=spf1 a mx ptr ~all"}}}
	if got := CheckSPF(context.Background(), dns, "example.com", "").Code; got != CodeSPFTooManyLookups {
		t.Fatalf("code %s, want %s", got, CodeSPFTooManyLookups)
	}
}

func TestDiagnoseDMARCMissing(t *testing.T) {
	rc := CheckDMARC(context.Background(), fakeDNS{}, "example.com")
	if rc.Code != "dmarc_missing" || !strings.Contains(rc.Message, "_dmarc.example.com") {
		t.Fatalf("got %+v", rc)
	}
}

func TestDiagnoseDKIMProxied(t *testing.T) {
	name := "resend._domainkey.example.com"
	cases := []struct {
		name string
		dns  DNSLookup
		code string
	}{
		{"proxied", fakeDNS{host: map[string][]string{name: {"104.21.5.9", "2606:4700::6810:1"}}}, CodeDKIMProxied},
		{"real cname", fakeDNS{cname: map[string]string{name: "x.dkim.example.net."}}, "dkim_found"},
		{"non cloudflare address", fakeDNS{host: map[string][]string{name: {"203.0.113.9"}}}, "dkim_not_found"},
		{"resolver cannot tell", txtOnlyDNS{fakeDNS{host: map[string][]string{name: {"104.21.5.9"}}}}, "dkim_not_found"},
	}
	for _, c := range cases {
		if got := CheckDomain(context.Background(), c.dns, "example.com", BackendResend).Records[1].Code; got != c.code {
			t.Errorf("%s: code %s, want %s", c.name, got, c.code)
		}
	}
}

func TestDiagnoseAlignment(t *testing.T) {
	cases := []struct {
		name, from, spf, dkim, code string
	}{
		{"exact", "example.com", "example.com", "", CodeAligned},
		{"dkim subdomain", "example.com", "bounce.other.net", "mail.example.com", CodeAligned},
		{"parent domain", "app.example.com", "", "example.com", CodeAligned},
		{"neither", "example.com", "amazonses.com", "provider.net", CodeNotAligned},
		{"suffix lookalike", "example.com", "notexample.com", "", CodeNotAligned},
		{"nothing known", "example.com", "", "", CodeNotAligned},
	}
	for _, c := range cases {
		if got := CheckAlignment(c.from, c.spf, c.dkim).Code; got != c.code {
			t.Errorf("%s: code %s, want %s", c.name, got, c.code)
		}
	}
}
