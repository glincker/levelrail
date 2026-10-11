package tlsowner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestResolve(t *testing.T) {
	trusted := &store.ProxyRouteStatus{Written: true, CertTrusted: true}
	untrusted := &store.ProxyRouteStatus{Written: true}
	tests := []struct {
		name string
		in   Inputs
		want Owner
	}{
		{"acme", Inputs{ACMEEnabled: true}, ACME},
		{"internal when acme off", Inputs{}, Internal},
		{"custom beats acme", Inputs{ACMEEnabled: true, Custom: true}, Custom},
		{"upstream beats acme", Inputs{Upstream: true, ACMEEnabled: true}, Proxy},
		{"upstream beats custom", Inputs{Upstream: true, Custom: true}, Proxy},
		{"trusted route", Inputs{ACMEEnabled: true, Route: trusted}, Proxy},
		{"trusted route beats custom", Inputs{Custom: true, Route: trusted}, Proxy},
		{"untrusted route ignored", Inputs{ACMEEnabled: true, Route: untrusted}, ACME},
		{"untrusted route, internal", Inputs{Route: untrusted}, Internal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Resolve(tc.in); got != tc.want {
				t.Fatalf("Resolve = %q, want %q", got, tc.want)
			}
		})
	}
}

type fakeSettings struct{ s store.IngressSettings }

func (f fakeSettings) GetIngressSettings(context.Context) (store.IngressSettings, error) {
	return f.s, nil
}

type fakeRoutes struct {
	rows []store.ProxyRouteStatus
	err  error
}

func (f fakeRoutes) ListProxyRouteStatus(context.Context) ([]store.ProxyRouteStatus, error) {
	return f.rows, f.err
}

func TestSnapshot(t *testing.T) {
	end := time.Now().Add(60 * 24 * time.Hour)
	routes := fakeRoutes{rows: []store.ProxyRouteStatus{
		{Domain: "App.example.com", Written: true, CertTrusted: true, CertIssuer: "R11", CertNotAfter: end},
		{Domain: "other.example.com", Written: true},
	}}
	ctx := context.Background()
	t.Run("upstream suppresses all stored certs", func(t *testing.T) {
		s := Load(ctx, fakeSettings{store.IngressSettings{ACMEEnabled: true, TLSTerminatedUpstream: true}}, routes, nil)
		if !s.Suppressed("anything.example.com") || s.Owner("other.example.com") != Proxy {
			t.Fatal("expected upstream to own every host")
		}
		c, ok := s.ProxyCert("app.example.com")
		if !ok || c.Issuer != "R11" || !c.NotAfter.Equal(end) {
			t.Fatalf("proxy cert = %+v, %v", c, ok)
		}
		if _, ok := s.ProxyCert("other.example.com"); ok {
			t.Fatal("untrusted route must not yield a certificate")
		}
	})
	t.Run("route only", func(t *testing.T) {
		s := Load(ctx, fakeSettings{store.IngressSettings{ACMEEnabled: true}}, routes, nil)
		if s.Owner("app.example.com") != Proxy || s.Owner("other.example.com") != ACME {
			t.Fatalf("owners: %s %s", s.Owner("app.example.com"), s.Owner("other.example.com"))
		}
		if s.Suppressed("other.example.com") {
			t.Fatal("acme host must not be suppressed")
		}
	})
	t.Run("read errors degrade to the ingress's own", func(t *testing.T) {
		s := Load(ctx, nil, fakeRoutes{err: errors.New("boom")}, map[string]bool{"C.example.com": true})
		if s.Owner("c.example.com") != Custom || s.Owner("x.example.com") != Internal {
			t.Fatal("unexpected owners")
		}
	})
}
