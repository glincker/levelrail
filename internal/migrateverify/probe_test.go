package migrateverify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func probeOpts(allowPrivate bool) ProbeOptions {
	return ProbeOptions{Timeout: 3 * time.Second, Concurrency: 2, AllowPrivate: allowPrivate}
}

func TestProbeReportsStatusAndLatency(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "app.example.test" {
			t.Errorf("host header = %q", r.Host)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	res := Prober{Opts: probeOpts(true)}.Probe(context.Background(), "app.example.test", srv.Listener.Addr().String())
	if res.Error != "" || res.Status != http.StatusNoContent || res.TLS {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestProbeTLSInvalidStillReportsStatus(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()

	res := Prober{Opts: probeOpts(true)}.Probe(context.Background(), "app.example.test", srv.Listener.Addr().String())
	if res.Status != http.StatusOK || !res.TLS || res.TLSValid || res.TLSError == "" {
		t.Fatalf("want status 200 with an invalid certificate, got %+v", res)
	}
}

func TestProbeRedirectIsReportedNeverFollowed(t *testing.T) {
	var elsewhereHits atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { elsewhereHits.Add(1) }))
	defer elsewhere.Close()

	tests := []struct {
		name     string
		location string
		offFam   bool
	}{
		{"other site by address", "http://" + elsewhere.Listener.Addr().String() + "/x?token=abc", true},
		{"other site by name", "https://evil.other.test/", true},
		{"same family", "https://www.example.test/home?x=1", false},
		{"relative", "/login", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, tc.location, http.StatusFound)
			}))
			defer srv.Close()
			res := Prober{Opts: probeOpts(true)}.Probe(context.Background(), "app.example.test", srv.Listener.Addr().String())
			if res.Status != http.StatusFound {
				t.Fatalf("status = %d, want 302 (redirect must not be followed)", res.Status)
			}
			if res.RedirectOffDomain != tc.offFam {
				t.Errorf("off domain = %v, want %v (target %q)", res.RedirectOffDomain, tc.offFam, res.RedirectTarget)
			}
			if strings.Contains(res.RedirectTarget, "token") || strings.Contains(res.RedirectTarget, "?") {
				t.Errorf("redirect target kept the query: %q", res.RedirectTarget)
			}
		})
	}
	if n := elsewhereHits.Load(); n != 0 {
		t.Fatalf("the off-domain redirect target was requested %d times", n)
	}
}

func TestProbeRefusesPrivateAndLoopbackByDefault(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()

	tests := []struct {
		name     string
		override string
		allow    bool
		blocked  bool
	}{
		{"loopback refused", srv.Listener.Addr().String(), false, true},
		{"loopback allowed by flag", srv.Listener.Addr().String(), true, false},
		{"private refused", "10.0.0.5:8080", false, true},
		{"metadata refused even with the flag", "169.254.169.254:80", true, true},
		{"link local v6 refused even with the flag", "[fe80::1]:80", true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := hits.Load()
			res := Prober{Opts: probeOpts(tc.allow)}.Probe(context.Background(), "app.example.test", tc.override)
			gotBlocked := strings.Contains(res.Error, ErrBlockedAddress.Error())
			if gotBlocked != tc.blocked {
				t.Fatalf("blocked = %v, want %v (result %+v)", gotBlocked, tc.blocked, res)
			}
			if tc.blocked && hits.Load() != before {
				t.Fatal("a blocked probe reached the server")
			}
		})
	}
}

func TestProbeLiteralLoopbackDomainRefused(t *testing.T) {
	res := Prober{Opts: probeOpts(false)}.Probe(context.Background(), "127.0.0.1", "")
	if !strings.Contains(res.Error, ErrBlockedAddress.Error()) {
		t.Fatalf("a literal loopback domain must be refused, got %+v", res)
	}
}

func TestProbeAllHonoursConcurrencyAndOrder(t *testing.T) {
	var inflight, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := inflight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		inflight.Add(-1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var pairs []Pair
	for _, d := range []string{"a.example.test", "b.example.test", "c.example.test", "d.example.test", "e.example.test", "f.example.test"} {
		pairs = append(pairs, Pair{App: "app", Domain: d})
	}
	opts := probeOpts(true)
	opts.TargetAddr, opts.SourceAddr = srv.Listener.Addr().String(), srv.Listener.Addr().String()
	got := Prober{Opts: opts}.ProbeAll(context.Background(), pairs)
	for i, p := range got {
		if p.Domain != pairs[i].Domain || p.Source.Status != 200 || p.Target.Status != 200 {
			t.Fatalf("pair %d: %+v", i, p)
		}
	}
	if int(peak.Load()) > opts.Concurrency {
		t.Fatalf("peak in flight = %d, limit %d", peak.Load(), opts.Concurrency)
	}
}

func TestProbeTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()
	opts := probeOpts(true)
	opts.Timeout = 100 * time.Millisecond
	res := Prober{Opts: opts}.Probe(context.Background(), "app.example.test", srv.Listener.Addr().String())
	if res.Error == "" || res.Status != 0 {
		t.Fatalf("want a timeout error, got %+v", res)
	}
}

func TestSameDomainFamily(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"example.com", "example.com", true},
		{"example.com", "www.example.com", true},
		{"a.example.com", "b.example.com", true},
		{"example.com", "example.org", false},
		{"example.com", "evilexample.com", false},
		{"127.0.0.1", "127.0.0.2", false},
		{"localhost", "other", false},
	}
	for _, tc := range tests {
		if got := SameDomainFamily(tc.a, tc.b); got != tc.want {
			t.Errorf("SameDomainFamily(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestValidateAddrAndDomain(t *testing.T) {
	for addr, ok := range map[string]bool{"": true, "203.0.113.5": true, "203.0.113.5:8443": true, "[2001:db8::1]:443": true, "2001:db8::1": true, "example.com": false, "http://1.2.3.4": false} {
		if err := ValidateAddr(addr); (err == nil) != ok {
			t.Errorf("ValidateAddr(%q) err = %v, want ok=%v", addr, err, ok)
		}
	}
	for d, ok := range map[string]bool{"app.example.com": true, "*.example.com": false, "a/b.com": false, "host:80": false, "localhost": false, "": false} {
		if ValidDomain(d) != ok {
			t.Errorf("ValidDomain(%q) = %v, want %v", d, !ok, ok)
		}
	}
}

func TestClampProbeOptions(t *testing.T) {
	got := ClampProbeOptions(ProbeOptions{Timeout: time.Hour, Concurrency: 9999})
	if got.Timeout != MaxProbeTimeout || got.Concurrency != MaxProbeConcurrency {
		t.Fatalf("not clamped: %+v", got)
	}
	got = ClampProbeOptions(ProbeOptions{})
	if got.Timeout != DefaultProbeTimeout || got.Concurrency != DefaultProbeConcurrency {
		t.Fatalf("defaults not applied: %+v", got)
	}
}
