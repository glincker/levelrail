package proxyroutes

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProbe(t *testing.T) {
	tests := []struct {
		name          string
		handler       http.HandlerFunc
		trust         bool
		wantReachable bool
		wantErr       string
	}{
		{"ok and trusted", func(w http.ResponseWriter, r *http.Request) {
			if r.Host != "example.com" {
				http.Error(w, "wrong host", http.StatusBadRequest)
			}
		}, true, true, ""},
		{"ok but untrusted cert", func(http.ResponseWriter, *http.Request) {}, false, true, "not trusted"},
		{"traefik 404", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprintln(w, "404 page not found")
		}, true, false, "no route"},
		{"app 404 counts as reachable", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "nope", http.StatusNotFound)
		}, true, true, ""},
		{"bad gateway", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }, true, false, "could not reach this server"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewTLSServer(tc.handler)
			defer srv.Close()
			target := ProbeTarget{Domain: "example.com", Address: srv.Listener.Addr().String(), Path: "/", Timeout: 5 * time.Second, Roots: x509.NewCertPool()}
			if tc.trust {
				target.Roots.AddCert(srv.Certificate())
			}
			got := Probe(context.Background(), target)
			if got.Reachable != tc.wantReachable || (tc.wantErr == "") != (got.Err == "") || !strings.Contains(got.Err, tc.wantErr) {
				t.Errorf("Probe() = %+v", got)
			}
			if got.CertIssuer == "" || got.CertNotAfter.IsZero() {
				t.Errorf("issuer = %q not_after = %v", got.CertIssuer, got.CertNotAfter)
			}
		})
	}
}

func TestProbe_Unreachable(t *testing.T) {
	got := Probe(context.Background(), ProbeTarget{Domain: "example.com", Address: "127.0.0.1:1", Path: "/", Timeout: time.Second})
	if got.Reachable || got.Err == "" {
		t.Errorf("Probe() = %+v", got)
	}
}

func TestRouterLoaded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/api/http/routers/acme-managed-a_b_com@file":
			_, _ = fmt.Fprint(w, `{"status":"enabled"}`)
		case "/api/http/routers/acme-managed-c_d_com@file":
			_, _ = fmt.Fprint(w, `{"status":"disabled"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	for router, want := range map[string]bool{"acme-managed-a_b_com@file": true, "acme-managed-c_d_com@file": false, "missing@file": false} {
		got, err := RouterLoaded(context.Background(), srv.URL, router, time.Second)
		if err != nil || got != want {
			t.Errorf("RouterLoaded(%s) = %v, %v", router, got, err)
		}
	}
}
