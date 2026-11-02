package deployurls

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type optOutStore struct {
	*fakeStore
	off map[string]bool
}

func (o optOutStore) ListAppAutoHostOptOuts(context.Context) (map[string]bool, error) {
	return o.off, nil
}

func TestSyncOptedOutAppGetsNoHostOrCopyDomain(t *testing.T) {
	f := newFake()
	app := store.DesiredService{Name: "web", Image: "web:ccccccc", Port: 80}
	fourDeploys(f, app)
	s := newSyncer(f, Config{Keep: 3, IdleMinutes: 5, MaxCopies: 2})
	s.Store = optOutStore{fakeStore: f, off: map[string]bool{"web": true}}
	if err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, r := range f.rows {
		if r.Host != "" {
			t.Fatalf("opted-out app copy has host %q", r.Host)
		}
	}
	if len(f.rows) != 0 {
		t.Fatalf("rows = %+v, want no copy when the public mode has no host", f.rows)
	}
}

func TestSyncWithoutOptOutKeepsHosts(t *testing.T) {
	f := newFake()
	fourDeploys(f, store.DesiredService{Name: "web", Image: "web:ccccccc", Port: 80})
	s := newSyncer(f, Config{Keep: 3, IdleMinutes: 5, MaxCopies: 2})
	s.Store = optOutStore{fakeStore: f}
	if err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.rows) == 0 {
		t.Fatal("default install lost its copies")
	}
}
