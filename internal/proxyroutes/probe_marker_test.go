package proxyroutes

import (
	"context"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbe_Marker(t *testing.T) {
	marker := &Marker{Header: "X-Check", Value: "token", ReplyHeader: "X-Reply", Reply: "secret-reply"}
	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    bool
	}{
		{"ingress answers", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Check") == "token" {
				w.Header().Set("X-Reply", "secret-reply")
				w.WriteHeader(http.StatusNoContent)
			}
		}, true},
		{"app answers instead", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("old container"))
		}, false},
		{"app echoes the request header", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Reply", r.Header.Get("X-Check"))
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewTLSServer(tc.handler)
			defer srv.Close()
			roots := x509.NewCertPool()
			roots.AddCert(srv.Certificate())
			got := Probe(context.Background(), ProbeTarget{
				Domain: "example.com", Address: srv.Listener.Addr().String(), Path: "/",
				Timeout: 5 * time.Second, Roots: roots, Marker: marker,
			})
			if got.Delivered != tc.want {
				t.Errorf("Delivered = %v, want %v (%+v)", got.Delivered, tc.want, got)
			}
		})
	}
}
