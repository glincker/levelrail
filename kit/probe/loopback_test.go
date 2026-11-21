package probe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func loopbackProber() *Prober { return New(nil, nil, Limits{}, WithLoopbackOnly()) }

func TestLoopbackOnly_RefusesNonLoopbackDial(t *testing.T) {
	for _, addr := range []string{"192.0.2.1:80", "[2001:db8::1]:80"} {
		for _, cfg := range []Config{{TCP: true}, {Path: "/"}} {
			start := time.Now()
			err := loopbackProber().Check(context.Background(), Target{Addr: addr}, cfg)
			if err == nil || !strings.Contains(err.Error(), "loopback") {
				t.Errorf("%s %+v: want loopback refusal, got %v", addr, cfg, err)
			}
			if time.Since(start) > time.Second {
				t.Errorf("%s: refusal must be immediate", addr)
			}
		}
	}
}

func TestLoopbackOnly_AllowsLoopbackForms(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "[::1]", "[::ffff:127.0.0.1]"} {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if host == "[::1]" {
			ln, err = net.Listen("tcp", "[::1]:0")
		}
		if err != nil {
			t.Skipf("%s: %v", host, err)
		}
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		srv.Listener = ln
		srv.Start()
		_, port, _ := net.SplitHostPort(ln.Addr().String())
		if err := loopbackProber().Check(context.Background(), Target{Addr: host + ":" + port}, Config{Path: "/"}); err != nil {
			t.Errorf("%s: %v", host, err)
		}
		srv.Close()
	}
}

func TestLoopbackOnly_RefusesRedirectToNonLoopback(t *testing.T) {
	for _, loc := range []string{"http://192.0.2.1/", "http://localhost/"} {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			http.Redirect(w, r, loc, http.StatusFound)
		}))
		start := time.Now()
		err := loopbackProber().Check(context.Background(), Target{Addr: strings.TrimPrefix(srv.URL, "http://")}, Config{Path: "/"})
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), "loopback") {
			t.Errorf("%s: want refusal, got %v", loc, err)
		}
		if hits.Load() != 1 || time.Since(start) > time.Second {
			t.Errorf("%s: hits %d, must refuse before connecting", loc, hits.Load())
		}
	}
}

type captureRemote struct{ cfg Config }

func (c *captureRemote) CheckRemote(_ context.Context, _ string, cfg Config) error {
	c.cfg = cfg
	return nil
}

func TestRemoteReceivesResolvedLimits(t *testing.T) {
	r := &captureRemote{}
	p := New(nil, nil, Limits{DefaultTimeout: 7 * time.Second, MaxRedirects: 3})
	if err := p.Check(context.Background(), Target{Addr: "127.0.0.1:1", Remote: r}, Config{Path: "/"}); err != nil {
		t.Fatal(err)
	}
	if r.cfg.Timeout != 7*time.Second || r.cfg.MaxRedirects != 3 {
		t.Fatalf("cfg %+v", r.cfg)
	}
	_ = p.Check(context.Background(), Target{Addr: "127.0.0.1:1", Remote: r}, Config{Path: "/", Timeout: time.Second})
	if r.cfg.Timeout != time.Second {
		t.Fatalf("explicit timeout lost: %+v", r.cfg)
	}
}

type unsupportedRemote struct{}

func (unsupportedRemote) CheckRemote(context.Context, string, Config) error {
	return ErrRemoteUnsupported
}

func TestRemoteUnsupportedFallsBackToLocalDial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			defer func() { _ = c.Close() }()
		}
	}()
	if err := New(nil, nil, Limits{}).Check(context.Background(), Target{Addr: ln.Addr().String(), Remote: unsupportedRemote{}}, Config{TCP: true}); err != nil {
		t.Fatal(err)
	}
}
