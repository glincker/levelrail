package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeProxyRouteStatuses struct {
	rows []store.ProxyRouteStatus
	err  error
}

func (f fakeProxyRouteStatuses) GetProxyIntegrationSettings(context.Context) (store.ProxyIntegrationSettings, error) {
	return store.ProxyIntegrationSettings{}, nil
}
func (f fakeProxyRouteStatuses) UpdateProxyIntegrationSettings(context.Context, store.ProxyIntegrationSettings) error {
	return nil
}
func (f fakeProxyRouteStatuses) ListProxyRouteStatus(context.Context) ([]store.ProxyRouteStatus, error) {
	return f.rows, f.err
}
func (f fakeProxyRouteStatuses) SaveProxyRouteVerification(context.Context, store.ProxyRouteStatus) error {
	return nil
}

func TestClassifyServedCert(t *testing.T) {
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	window := 14 * 24 * time.Hour
	tests := []struct {
		name     string
		notAfter time.Time
		want     string
	}{
		{name: "ninety days left", notAfter: now.Add(90 * 24 * time.Hour), want: "healthy"},
		{name: "inside the window", notAfter: now.Add(5 * 24 * time.Hour), want: "expiring_soon"},
		{name: "already past", notAfter: now.Add(-time.Hour), want: "expired"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyServedCert(tc.notAfter, now, window); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProxyServedCerts(t *testing.T) {
	end := time.Date(2027, 1, 9, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		deps *proxyIntegrationDeps
		want int
	}{
		{name: "no integration", deps: nil, want: 0},
		{name: "trusted cert is used", deps: &proxyIntegrationDeps{store: fakeProxyRouteStatuses{rows: []store.ProxyRouteStatus{{Domain: "A.example.com", CertTrusted: true, CertNotAfter: end}}}}, want: 1},
		{name: "untrusted default cert is ignored", deps: &proxyIntegrationDeps{store: fakeProxyRouteStatuses{rows: []store.ProxyRouteStatus{{Domain: "a.example.com", CertTrusted: false, CertNotAfter: end}}}}, want: 0},
		{name: "store error is tolerated", deps: &proxyIntegrationDeps{store: fakeProxyRouteStatuses{err: errors.New("boom")}}, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{proxyIntegration: tc.deps}
			got := rt.proxyServedCerts(context.Background())
			if len(got) != tc.want {
				t.Fatalf("len = %d, want %d", len(got), tc.want)
			}
			if tc.want == 1 {
				if _, ok := got["a.example.com"]; !ok {
					t.Errorf("domain not lower-cased: %v", got)
				}
			}
		})
	}
}
