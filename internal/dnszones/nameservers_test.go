package dnszones

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClassifyDelegation(t *testing.T) {
	exp := []string{"Ana.NS.Cloudflare.com.", "bob.ns.cloudflare.com"}
	cases := []struct {
		name    string
		private bool
		current []string
		err     error
		want    string
		reason  string
	}{
		{"all match with case and dots", false, []string{"ana.ns.cloudflare.com", "BOB.ns.cloudflare.com."}, nil, DelegatedYes, ""},
		{"subset still delegates", false, []string{"ana.ns.cloudflare.com."}, nil, DelegatedYes, ""},
		{"none match", false, []string{"ns1.registrar.example", "ns2.registrar.example"}, nil, DelegatedNo, ""},
		{"some match", false, []string{"ana.ns.cloudflare.com", "ns1.registrar.example"}, nil, DelegatedPartial, ""},
		{"lookup failed", false, nil, errors.New("timeout"), DelegatedUnknown, DelegationReasonLookupFailed},
		{"private zone", true, []string{"ana.ns.cloudflare.com"}, nil, DelegatedUnknown, DelegationReasonPrivateZone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := ClassifyDelegation(c.private, exp, c.current, c.err)
			if d.Delegated != c.want || d.Reason != c.reason {
				t.Fatalf("got %+v, want %s/%s", d, c.want, c.reason)
			}
			if d.Expected[0] != "ana.ns.cloudflare.com" {
				t.Errorf("expected not normalized: %v", d.Expected)
			}
		})
	}
	if d := ClassifyDelegation(false, exp, []string{"ns1.registrar.example."}, nil); d.Current[0] != "ns1.registrar.example" {
		t.Errorf("current not normalized: %v", d.Current)
	}
}

type countingQuerier struct {
	n      atomic.Int32
	values []string
	fail   bool
}

func (q *countingQuerier) Query(_ context.Context, server, _ string, _ uint16) Answer {
	q.n.Add(1)
	if q.fail {
		return Answer{Server: server, Error: "i/o timeout"}
	}
	return Answer{Server: server, Values: q.values}
}

func TestNSCheckerCachesAndSkipsPrivate(t *testing.T) {
	q := &countingQuerier{values: []string{"ns1.registrar.example."}}
	c := NewNSChecker(q, NSCheckerOptions{Servers: []string{"1.1.1.1:53"}, Timeout: time.Second, CacheTTL: time.Minute, Concurrency: 2})
	z := Zone{Name: "example.com", NameServers: []string{"ana.ns.cloudflare.com"}}
	for range 3 {
		if d := c.Check(context.Background(), z); d.Delegated != DelegatedNo || d.Current[0] != "ns1.registrar.example" {
			t.Fatalf("check = %+v", d)
		}
	}
	if q.n.Load() != 1 {
		t.Errorf("queries = %d, want 1 (cached)", q.n.Load())
	}
	z.Private = true
	if d := c.Check(context.Background(), z); d.Reason != DelegationReasonPrivateZone || q.n.Load() != 1 {
		t.Errorf("private zone looked up: %+v", d)
	}
	fq := &countingQuerier{fail: true}
	fc := NewNSChecker(fq, NSCheckerOptions{Servers: []string{"1.1.1.1:53", "8.8.8.8:53"}, Timeout: time.Second, CacheTTL: time.Minute})
	if d := fc.Check(context.Background(), Zone{Name: "example.org", NameServers: []string{"a.example"}}); d.Delegated != DelegatedUnknown {
		t.Errorf("failed lookup = %+v", d)
	}
}

const rdapDomainBody = `{"objectClassName":"domain","entities":[
 {"roles":["registrant"],"vcardArray":["vcard",[["fn",{},"text","Someone"]]]},
 {"roles":["registrar"],"vcardArray":["vcard",[["version",{},"text","4.0"],["fn",{},"text","Example Registrar, Inc."],["url",{},"uri","https://registrar.example"]]]}]}`

func TestParseRDAPRegistrar(t *testing.T) {
	h, ok := ParseRDAPRegistrar([]byte(rdapDomainBody))
	if !ok || h.Name != "Example Registrar, Inc." || h.URL != "https://registrar.example" {
		t.Fatalf("hint = %+v %v", h, ok)
	}
	for _, bad := range []string{`not json`, `{"entities":[]}`, `{"entities":[{"roles":["registrar"],"vcardArray":["vcard","oops"]}]}`} {
		if _, ok := ParseRDAPRegistrar([]byte(bad)); ok {
			t.Errorf("parsed %q", bad)
		}
	}
	h, _ = ParseRDAPRegistrar([]byte(strings.Replace(rdapDomainBody, "https://registrar.example", "javascript:alert(1)", 1)))
	if h.URL != "" {
		t.Errorf("non https url kept: %q", h.URL)
	}
}

func TestRDAPLookupAndFailureTolerance(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/boot":
			_, _ = w.Write([]byte(`{"services":[[["com"],["` + srv.URL + `/com/"]],[["net"],["http://insecure.example/"]]]}`))
		case "/com/domain/example.com":
			_, _ = w.Write([]byte(rdapDomainBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	r := NewRDAP(srv.Client(), RDAPOptions{Timeout: time.Second, CacheTTL: time.Minute, MaxBytes: 1 << 16})
	r.BootstrapURL = srv.URL + "/boot"
	if h, ok := r.Registrar(context.Background(), "Example.com."); !ok || h.Name != "Example Registrar, Inc." {
		t.Fatalf("hint = %+v %v", h, ok)
	}
	for _, d := range []string{"missing.com", "example.net", "example.zz", "localhost"} {
		if _, ok := r.Registrar(context.Background(), d); ok {
			t.Errorf("%s returned a hint", d)
		}
	}
	small := NewRDAP(srv.Client(), RDAPOptions{Timeout: time.Second, CacheTTL: time.Minute, MaxBytes: 10})
	small.BootstrapURL = srv.URL + "/boot"
	if _, ok := small.Registrar(context.Background(), "example.com"); ok {
		t.Error("oversized response accepted")
	}
}

func TestSplitTXTChunks(t *testing.T) {
	long := strings.Repeat("a", 600)
	parts := splitTXT(long)
	if len(parts) != 3 || len(parts[0]) != 255 || len(parts[2]) != 90 {
		t.Fatalf("parts = %d", len(parts))
	}
	if got := unquoteTXT(quoteTXT(long)); got != long {
		t.Errorf("round trip lost text")
	}
}
