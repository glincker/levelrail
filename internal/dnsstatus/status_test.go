package dnsstatus

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func prio(n int) *int { return &n }

type fakeResolver struct {
	txt   map[string][]string
	mx    map[string][]*net.MX
	hosts map[string][]string
	cname map[string]string
	slow  map[string]bool
	calls atomic.Int32
}

var errNX = &net.DNSError{Err: "no such host", IsNotFound: true}

func (f *fakeResolver) wait(ctx context.Context, name string) error {
	f.calls.Add(1)
	if f.slow[name] {
		<-ctx.Done()
		return &net.DNSError{Err: "i/o timeout", IsTimeout: true}
	}
	return nil
}

func (f *fakeResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	if err := f.wait(ctx, name); err != nil {
		return nil, err
	}
	if v, ok := f.txt[name]; ok {
		return v, nil
	}
	return nil, errNX
}

func (f *fakeResolver) LookupCNAME(_ context.Context, name string) (string, error) {
	if c, ok := f.cname[name]; ok {
		return c, nil
	}
	return name + ".", nil
}

func (f *fakeResolver) LookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	if err := f.wait(ctx, name); err != nil {
		return nil, err
	}
	if v, ok := f.mx[name]; ok {
		return v, nil
	}
	return nil, errNX
}

func (f *fakeResolver) LookupHost(ctx context.Context, name string) ([]string, error) {
	if err := f.wait(ctx, name); err != nil {
		return nil, err
	}
	if v, ok := f.hosts[name]; ok {
		return v, nil
	}
	return nil, errNX
}

func (f *fakeResolver) LookupSRV(context.Context, string, string, string) (string, []*net.SRV, error) {
	return "", nil, errNX
}

func byID(s Status) map[string]Record {
	out := map[string]Record{}
	for _, r := range s.Records {
		out[r.ID] = r
	}
	return out
}

func TestFromResolverClassification(t *testing.T) {
	r := &fakeResolver{
		txt: map[string][]string{
			"example.com":        {"google-site-verification=abc", "v=spf1 mx ~all"},
			"_dmarc.example.com": {"v=DMARC1; p=none"},
		},
		mx:    map[string][]*net.MX{"example.com": {{Host: "Mail.Example.com.", Pref: 10}}},
		hosts: map[string][]string{"mail.example.com": {"203.0.113.9"}},
		slow:  map[string]bool{"_mta-sts.example.com": true},
	}
	wants := []Want{
		{ID: "mx", Type: "MX", Name: "example.com", Value: "mail.example.com", Priority: prio(10)},
		{ID: "spf", Type: "TXT", Name: "example.com", Value: "v=spf1 mx -all"},
		{ID: "dmarc", Type: "TXT", Name: "_dmarc.example.com", Value: "v=DMARC1; p=none"},
		{ID: "dkim", Type: "TXT", Name: "s1._domainkey.example.com", Value: "v=DKIM1; k=rsa; p=AAAA"},
		{ID: "host", Type: "A", Name: "mail.example.com", Value: "203.0.113.10"},
		{ID: "sts", Type: "TXT", Name: "_mta-sts.example.com", Value: "v=STSv1; id=1"},
	}
	st := FromResolver(context.Background(), r, wants, Options{Timeout: 50 * time.Millisecond, Concurrency: 2}, time.Unix(0, 0))
	got := byID(st)
	cases := map[string]string{"mx": Live, "spf": Different, "dmarc": Live, "dkim": Waiting, "host": Different, "sts": Unknown}
	for id, want := range cases {
		if got[id].Status != want {
			t.Errorf("%s = %s, want %s (%+v)", id, got[id].Status, want, got[id])
		}
	}
	if f := got["spf"].Found; len(f) != 1 || f[0].Value != "v=spf1 mx ~all" {
		t.Fatalf("different must show only the owned SPF value: %+v", f)
	}
	if got["host"].Found[0].Value != "203.0.113.9" || got["host"].Reason == "" {
		t.Fatalf("different A must show what exists: %+v", got["host"])
	}
	if st.ProviderSource != SourcePublicDNS || st.Counts != (Counts{Live: 2, Waiting: 1, Different: 2, Unknown: 1, Total: 6}) {
		t.Fatalf("counts = %+v source %s", st.Counts, st.ProviderSource)
	}
	if st.CheckedAt == "" || got["mx"].CheckedAt != st.CheckedAt {
		t.Fatal("checked_at must be set on the report and each record")
	}
}

func TestFromZoneProxiedConflictAndTXTChunks(t *testing.T) {
	sets := []ZoneSet{
		{Name: "mail.example.com", Type: "A", TTL: 1, Values: []string{"203.0.113.9"}, Proxied: true},
		{Name: "example.com.", Type: "MX", TTL: 300, Values: []string{"20 mail.example.com."}},
		{Name: "s1._domainkey.example.com", Type: "TXT", TTL: 300, Values: []string{`"v=DKIM1; k=rsa; p=ABC" "DEF"`}},
		{Name: "www.example.com", Type: "CNAME", TTL: 300, Values: []string{"Edge.Example.net."}},
	}
	wants := []Want{
		{ID: "host", Type: "A", Name: "mail.example.com", Value: "203.0.113.9"},
		{ID: "mx", Type: "MX", Name: "example.com", Value: "mail.example.com", Priority: prio(10)},
		{ID: "dkim", Type: "TXT", Name: "s1._domainkey.example.com", Value: "v=DKIM1; k=rsa; p=ABCDEF"},
		{ID: "www", Type: "CNAME", Name: "www.example.com", Value: "edge.example.net"},
	}
	st := FromZone(wants, sets, ZoneOptions{Shadow: map[string]bool{"mail.example.com": true}, ProxyAware: true}, time.Now())
	got := byID(st)
	if got["host"].Status != Conflict || got["host"].Found[0].Proxied == nil || !*got["host"].Found[0].Proxied {
		t.Fatalf("proxied mail host = %+v", got["host"])
	}
	if got["mx"].Status != Conflict {
		t.Fatalf("an MX whose target is proxied must conflict: %+v", got["mx"])
	}
	if got["dkim"].Status != Live || got["www"].Status != Live {
		t.Fatalf("chunked TXT and CNAME dots must match: %+v %+v", got["dkim"], got["www"])
	}
	st = FromZone(wants[1:2], sets, ZoneOptions{}, time.Now())
	if r := st.Records[0]; r.Status != Different || r.Reason != "MX to mail.example.com exists with priority 20 instead of 10." {
		t.Fatalf("MX priority = %+v", r)
	}
}

func TestNormalize(t *testing.T) {
	for _, c := range [][3]string{
		{"TXT", `"v=spf1 " "mx -all"`, "v=spf1 mx -all"},
		{"TXT", "V=SPF1  MX   -ALL", "v=spf1 mx -all"},
		{"MX", "10 Mail.Example.COM.", "10 mail.example.com"},
		{"CNAME", "Target.Example.net.", "target.example.net"},
		{"AAAA", "2001:DB8::0:1", "2001:db8::1"},
	} {
		if got := Normalize(c[0], c[1]); got != c[2] {
			t.Errorf("Normalize(%s, %q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
}

func TestCacheSingleFlightAndTTL(t *testing.T) {
	var c Cache
	var runs atomic.Int32
	release := make(chan struct{})
	fn := func() Status {
		runs.Add(1)
		<-release
		return Status{Records: []Record{{ID: "a"}}}
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); c.Do("k", 0, fn) }()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if runs.Load() != 1 {
		t.Fatalf("concurrent identical calls ran %d times, want 1", runs.Load())
	}
	c.Do("k", 0, fn)
	if runs.Load() != 2 {
		t.Fatal("ttl 0 must re-run once nothing is in flight")
	}
	c.Do("k", time.Minute, fn)
	c.Do("k", time.Minute, fn)
	if runs.Load() != 3 {
		t.Fatalf("a fresh result within ttl must be reused, ran %d", runs.Load())
	}
	t.Setenv(EnvCacheSeconds, "0")
	if CacheTTL() != 0 {
		t.Fatal("0 must disable the cache")
	}
}

func TestUnknownErr(t *testing.T) {
	if unknownErr(errNX) || unknownErr(errors.New("demo: no record")) {
		t.Fatal("not found is waiting, not unknown")
	}
	if !unknownErr(&net.DNSError{Err: "server misbehaving", IsTemporary: true}) || !unknownErr(context.DeadlineExceeded) {
		t.Fatal("SERVFAIL and timeouts are unknown")
	}
}
