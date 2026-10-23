package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestClassifyRoute(t *testing.T) {
	tests := []struct{ route, want string }{
		{"/", routeKindPage},
		{"/pricing", routeKindPage},
		{"/blog/:id", routeKindPage},
		{"/api/users/:id", routeKindAPI},
		{"/graphql", routeKindAPI},
		{"/_next/static/chunks/*", routeKindAsset},
		{"/favicon/site.webmanifest", routeKindAsset},
		{"/images/legal/privacy-choices.svg", routeKindAsset},
		{"/logo/GLIN-D.svg", routeKindAsset},
		{"/robots.txt", routeKindAsset},
		{"/Docs/Guide.PDF", routeKindAsset},
	}
	for _, tt := range tests {
		if got := classifyRoute(tt.route); got != tt.want {
			t.Errorf("classifyRoute(%q) = %q, want %q", tt.route, got, tt.want)
		}
	}
}

func TestTopRoutesKeepsPagesVisibleBesideAssets(t *testing.T) {
	b := telemetry.Breakdown{Routes: []telemetry.RouteRow{
		{Route: "/_next/static/chunks/*", Requests: 400},
		{Route: "/images/a.svg", Requests: 300},
		{Route: "/images/b.svg", Requests: 200},
		{Route: "/", Requests: 30},
		{Route: "/pricing", Requests: 20},
	}}
	rows := topRoutes(b, 2)
	pages := 0
	for _, r := range rows {
		if r.Kind == routeKindPage {
			pages++
		}
	}
	if pages != 2 || len(rows) != 4 {
		t.Fatalf("want 2 pages and 2 assets, got %d pages in %+v", pages, rows)
	}
}

func TestKindTotalsSumEveryRouteNotJustTheTop(t *testing.T) {
	b := telemetry.Breakdown{Routes: []telemetry.RouteRow{
		{Route: "/a.js", Requests: 10, Errors5xx: 5},
		{Route: "/b.js", Requests: 10},
		{Route: "/", Requests: 4},
	}}
	got := kindTotals(b)
	if got[routeKindAsset].Requests != 20 || got[routeKindAsset].ErrorRate5xx != 0.25 || got[routeKindPage].Requests != 4 {
		t.Fatalf("totals = %+v", got)
	}
}
