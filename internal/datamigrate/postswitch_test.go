package datamigrate

import (
	"context"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeResolver struct{ st DomainState }

func (f fakeResolver) Lookup(context.Context, string) DomainState { return f.st }

func statusOf(checks []Check, id string) string {
	for _, c := range checks {
		if c.ID == id {
			return c.Status
		}
	}
	return "missing"
}

func serverProber(t *testing.T, srv *httptest.Server, st DomainState, trust bool) Prober {
	t.Helper()
	p := Prober{
		Resolver: fakeResolver{st},
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, srv.Listener.Addr().String())
		},
	}
	if trust {
		pool := x509.NewCertPool()
		pool.AddCert(srv.Certificate())
		p.RootCAs = pool
	}
	return p
}

func TestPostSwitch(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	const domain = "example.com"
	here := DomainState{Addrs: []string{"203.0.113.10"}}
	targets := []string{"203.0.113.10"}

	tests := []struct {
		name   string
		status int
		trust  bool
		dns    DomainState
		want   map[string]string
	}{
		{"all pass", 200, true, here, map[string]string{"resolves": CheckPass, "certificate": CheckPass, "health": CheckPass}},
		{"dns still on source", 200, true, DomainState{Addrs: []string{"198.51.100.7"}}, map[string]string{"resolves": CheckFail, "certificate": CheckPass, "health": CheckPass}},
		{"untrusted certificate skips health", 200, false, here, map[string]string{"resolves": CheckPass, "certificate": CheckFail, "health": CheckFail}},
		{"unhealthy path", 503, true, here, map[string]string{"resolves": CheckPass, "certificate": CheckPass, "health": CheckFail}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status = tt.status
			checks := serverProber(t, srv, tt.dns, tt.trust).PostSwitch(t.Context(), domain, "/healthz", targets)
			for id, want := range tt.want {
				if got := statusOf(checks, id); got != want {
					t.Errorf("%s = %s, want %s (%+v)", id, got, want, checks)
				}
			}
		})
	}
}

func TestPostSwitchNothingListening(t *testing.T) {
	p := Prober{
		Resolver: fakeResolver{DomainState{Addrs: []string{"203.0.113.10"}}},
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, &net.OpError{Op: "dial", Err: context.DeadlineExceeded}
		},
	}
	checks := p.PostSwitch(t.Context(), "example.com", "/", []string{"203.0.113.10"})
	if got := statusOf(checks, "certificate"); got != CheckFail {
		t.Errorf("certificate = %s, want fail", got)
	}
}
