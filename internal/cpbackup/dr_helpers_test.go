package cpbackup

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/objectstore/objectstoretest"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeDest struct {
	buckets map[string]Bucket
	locs    map[string]Location
}

func (d fakeDest) Open(_ context.Context, id string) (Bucket, Location, error) {
	b, ok := d.buckets[id]
	if !ok {
		return nil, Location{}, fmt.Errorf("unknown destination %q", id)
	}
	return b, d.locs[id], nil
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type drEnv struct {
	t     *testing.T
	db    *store.DB
	svc   *Service
	srv   *objectstoretest.Server
	srv2  *objectstoretest.Server
	clock *fakeClock
	id    *age.X25519Identity
	dir   string
}

func newBucket(t *testing.T, srv *objectstoretest.Server) Bucket {
	t.Helper()
	c, err := objectstore.New(objectstore.Config{
		Endpoint: srv.URL, Bucket: srv.Bucket, AccessKeyID: "k", SecretAccessKey: "s", PathStyle: true,
		HTTPClient: &http.Client{Timeout: 10 * time.Second}, MaxAttempts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func newDREnv(t *testing.T) *drEnv {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(context.Background(), filepath.Join(dir, "levelrail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	srv, srv2 := objectstoretest.New("backups"), objectstoretest.New("escrow")
	t.Cleanup(srv.Close)
	t.Cleanup(srv2.Close)
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{t: time.Date(2026, 9, 25, 2, 0, 0, 0, time.UTC)}
	dest := fakeDest{
		buckets: map[string]Bucket{"main": newBucket(t, srv), "alias": newBucket(t, srv), "esc": newBucket(t, srv2)},
		locs: map[string]Location{
			"main":  {TargetID: "main", Name: "main", Endpoint: srv.URL, Bucket: "backups"},
			"alias": {TargetID: "alias", Name: "alias", Endpoint: srv.URL, Bucket: "backups"},
			"esc":   {TargetID: "esc", Name: "esc", Endpoint: srv2.URL, Bucket: "escrow"},
		},
	}
	svc := NewService(db, db, dest, dir, "v-test", DefaultOptions(), slog.New(slog.DiscardHandler))
	svc.Now = clock.Now
	return &drEnv{t: t, db: db, svc: svc, srv: srv, srv2: srv2, clock: clock, id: id, dir: dir}
}

func (e *drEnv) configure(mut func(*ConfigUpdate)) {
	e.t.Helper()
	u := ConfigUpdate{Enabled: true, TargetID: "main", Recipients: []string{e.id.Recipient().String()}}
	if mut != nil {
		mut(&u)
	}
	if err := e.svc.UpdateConfig(context.Background(), u); err != nil {
		e.t.Fatalf("UpdateConfig: %v", err)
	}
}

func (e *drEnv) settings() store.CPDRSettings {
	e.t.Helper()
	s, err := e.db.GetCPDRSettings(context.Background())
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

func (e *drEnv) backup() Manifest {
	e.t.Helper()
	m, err := e.svc.RunBackup(context.Background(), time.Time{})
	if err != nil {
		e.t.Fatalf("RunBackup: %v", err)
	}
	return m
}
