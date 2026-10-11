package proxyroutes

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type standaloneSyncStore struct {
	*fakeSyncStore
	standalone []store.StandaloneDomain
}

func (f *standaloneSyncStore) ListStandaloneDomains(context.Context) ([]store.StandaloneDomain, error) {
	return f.standalone, nil
}

func TestSync_StandaloneDomainsGetRouteFiles(t *testing.T) {
	dir := t.TempDir()
	st := &standaloneSyncStore{fakeSyncStore: newFakeSyncStore(dir)}
	st.services = []store.DesiredService{{Name: "web", Domains: []string{"a.example.com"}}}
	st.standalone = []store.StandaloneDomain{{Domain: "tools.example.com", Target: "127.0.0.1:3000"}, {Domain: "a.example.com"}}
	s := newTestSyncer(st)
	ctx := context.Background()

	plan, err := s.Plan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, it := range plan.Items {
		kinds = append(kinds, it.Domain+"="+it.Key())
	}
	want := []string{"a.example.com=app:web", "tools.example.com=standalone"}
	if !equalSet(kinds, want) {
		t.Fatalf("plan targets = %v, want %v (app wins a shared name)", kinds, want)
	}
	if _, err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if files := managedFiles(t, dir); !equalSet(files, []string{"acme-managed-a.example.com.yaml", "acme-managed-tools.example.com.yaml"}) {
		t.Fatalf("files = %v", files)
	}

	st.standalone = nil
	if _, err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if files := managedFiles(t, dir); !equalSet(files, []string{"acme-managed-a.example.com.yaml"}) {
		t.Fatalf("deleting the standalone domain must remove its file, got %v", files)
	}
}
