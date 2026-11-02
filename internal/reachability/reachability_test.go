package reachability

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeDialer struct{ open map[string]bool }

func (f fakeDialer) DialContext(_ context.Context, _, addr string) (net.Conn, error) {
	if f.open[addr] {
		c, s := net.Pipe()
		_ = s.Close()
		return c, nil
	}
	return nil, errors.New("refused")
}

func TestProbeLocalListening(t *testing.T) {
	d := fakeDialer{open: map[string]bool{"127.0.0.1:80": true, "10.0.0.5:80": true, "127.0.0.1:8080": true}}
	rs := ProbeLocal(t.Context(), d, []Target{{Name: "http", Port: 80}, {Name: "https", Port: 443}, {Name: "dashboard", Port: 8080}}, "10.0.0.5")
	if !rs[0].Listening || rs[1].Listening || !rs[2].Listening {
		t.Fatalf("listening = %v %v %v", rs[0].Listening, rs[1].Listening, rs[2].Listening)
	}
	if !rs[0].Exposable() || rs[2].Exposable() {
		t.Fatal("loopback-only dashboard must not be exposable")
	}
	if got := ExposablePorts(rs); len(got) != 1 || got[0] != 80 {
		t.Fatalf("exposable = %v", got)
	}
}

func TestBlockedPortAdvice(t *testing.T) {
	rs := []PortReport{{Target: Target{Name: "http", Port: 80}, Listening: true, Probes: []Probe{{Listening: true}}}}
	ApplyDialback(rs, DialbackResult{Unreachable: []int{80}})
	if rs[0].External != ExternalUnreachable || !strings.Contains(rs[0].Advice, "Open TCP port 80 inbound") {
		t.Fatalf("report = %+v", rs[0])
	}
}

func publicResolver(context.Context, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
}

func TestDialbackEmptyURLContactsNothing(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	if _, err := Dialback(t.Context(), "", []int{80}, DialbackOptions{Client: srv.Client()}); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 {
		t.Fatal("request made with empty url")
	}
}

func TestDialbackRequiresHTTPS(t *testing.T) {
	_, err := Dialback(t.Context(), "http://example.com/x", []int{80}, DialbackOptions{Resolve: publicResolver})
	if !errors.Is(err, ErrDialbackURL) {
		t.Fatalf("err = %v", err)
	}
}

func TestDialbackRefusesPrivateByDefault(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	_, err := Dialback(t.Context(), srv.URL, []int{80}, DialbackOptions{Client: srv.Client()})
	if !errors.Is(err, ErrDialbackURL) || hits.Load() != 0 {
		t.Fatalf("err = %v hits = %d", err, hits.Load())
	}
}

func TestDialbackAnswer(t *testing.T) {
	var body string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 64)
		n, _ := r.Body.Read(b)
		body = string(b[:n])
		_, _ = w.Write([]byte(`{"reachable":[80],"unreachable":[443]}`))
	}))
	defer srv.Close()
	res, err := Dialback(t.Context(), srv.URL, []int{80, 443}, DialbackOptions{AllowPrivate: true, Client: srv.Client()})
	if err != nil || len(res.Reachable) != 1 || res.Unreachable[0] != 443 {
		t.Fatalf("res = %+v err = %v", res, err)
	}
	if body != `{"ports":[80,443]}` {
		t.Fatalf("body = %s", body)
	}
}

func TestDialbackTimeout(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer srv.Close()
	c := srv.Client()
	c.Timeout = 50 * time.Millisecond
	if _, err := Dialback(t.Context(), srv.URL, []int{80}, DialbackOptions{AllowPrivate: true, Client: c}); err == nil {
		t.Fatal("want timeout error")
	}
}
