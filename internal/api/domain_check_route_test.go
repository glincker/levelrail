package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRouteStatusFor(t *testing.T) {
	at := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	rows := []store.ProxyRouteStatus{
		{Domain: "down.example.com", Reachable: true, StatusCode: 503, VerifiedAt: at},
		{Domain: "up.example.com", Reachable: true, StatusCode: 200, VerifiedAt: at},
		{Domain: "new.example.com"},
	}
	tests := []struct {
		name   string
		store  fakeProxyRouteStatuses
		domain string
		want   *routeStatusResource
	}{
		{"server error", fakeProxyRouteStatuses{rows: rows}, "DOWN.example.com", &routeStatusResource{Status: 503, OK: false, CheckedAt: "2026-11-06T10:00:00Z"}},
		{"ok", fakeProxyRouteStatuses{rows: rows}, "up.example.com", &routeStatusResource{Status: 200, OK: true, CheckedAt: "2026-11-06T10:00:00Z"}},
		{"never probed", fakeProxyRouteStatuses{rows: rows}, "new.example.com", nil},
		{"unknown domain", fakeProxyRouteStatuses{rows: rows}, "other.example.com", nil},
		{"store error", fakeProxyRouteStatuses{err: errors.New("boom")}, "up.example.com", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{proxyIntegration: &proxyIntegrationDeps{store: tc.store}}
			got := rt.routeStatusFor(context.Background(), tc.domain)
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
	if (&Router{}).routeStatusFor(context.Background(), "a.example.com") != nil {
		t.Fatal("no proxy integration must give no route")
	}
}
