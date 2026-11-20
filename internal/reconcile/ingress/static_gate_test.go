package ingress

import (
	"context"
	"log/slog"
	"testing"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

type gatedStore struct {
	*fakeStore
	hashes map[string]string
}

func (g *gatedStore) ListStaticSitePasswordHashes(context.Context) (map[string]string, error) {
	return g.hashes, nil
}

func TestController_Reconcile_StaticSite_PasswordGate(t *testing.T) {
	st := &gatedStore{
		fakeStore: &fakeStore{staticSites: []store.StaticSite{
			{Name: "docs", Domains: []string{"docs.example.com"}, RootDir: "/data/static/docs/abc"},
			{Name: "open", Domains: []string{"open.example.com"}, RootDir: "/data/static/open/abc"},
		}},
		hashes: map[string]string{"docs": "$2a$10$abcdefghijklmnopqrstuuabcdefghijklmnopqrstuvwxyz012345"},
	}
	applier := &fakeApplier{}
	c := New(st, newFakeRuntime(), applier, WithLogger(slog.New(slog.DiscardHandler)))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	routes := applier.routes(t)
	if len(routes) != 2 {
		t.Fatalf("routes = %d, want 2", len(routes))
	}
	gated := 0
	for _, r := range routes {
		if _, ok := r.Handle[0].(ingress.BasicAuthHandler); ok {
			gated++
		}
	}
	if gated != 1 {
		t.Fatalf("gated routes = %d, want exactly the protected site", gated)
	}
}
