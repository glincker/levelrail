package deploy

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

// interleaveStore runs hook once, right before the first desired-state save,
// so an env write lands between the deploy's snapshot read and its save.
type interleaveStore struct {
	*store.DB
	once sync.Once
	hook func()
}

func (s *interleaveStore) SaveDesiredService(ctx context.Context, svc store.DesiredService) error {
	s.once.Do(s.hook)
	return s.DB.SaveDesiredService(ctx, svc)
}

func openCASStore(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestImageTriggerDeployKeepsEnvWrittenAfterSnapshot(t *testing.T) {
	ctx := context.Background()
	db := openCASStore(t)
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "web:v1", Port: 80, Env: map[string]string{"A": "1"}}); err != nil {
		t.Fatal(err)
	}
	snap, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	st := &interleaveStore{DB: db, hook: func() {
		if _, err := db.MergeServiceEnv(ctx, "web", map[string]string{"B": "2"}, nil); err != nil {
			t.Error(err)
		}
	}}
	updated, err := ImageTrigger{Store: st}.Deploy(ctx, *snap, "web:v2")
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	got, _ := db.GetDesiredService(ctx, "web")
	if got.Image != "web:v2" || got.Env["A"] != "1" || got.Env["B"] != "2" {
		t.Fatalf("stored image %q env %v, want web:v2 with A and B", got.Image, got.Env)
	}
	if !got.EnvDirty || !updated.EnvDirty {
		t.Fatalf("env latch must stay set for keys written after the snapshot, got stored %v returned %v", got.EnvDirty, updated.EnvDirty)
	}
}

func TestImageTriggerDeployVsEnvMergesConcurrent(t *testing.T) {
	ctx := context.Background()
	db := openCASStore(t)
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "web:v0", Port: 80}); err != nil {
		t.Fatal(err)
	}
	const n = 20
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := db.MergeServiceEnv(ctx, "web", map[string]string{fmt.Sprintf("K%d", i): "v"}, nil); err != nil {
				t.Errorf("merge: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			snap, err := db.GetDesiredService(ctx, "web")
			if err != nil {
				t.Errorf("get: %v", err)
				return
			}
			// Exhausted retries may surface as a typed stale error; a lost key may not happen.
			_, _ = ImageTrigger{Store: db}.Deploy(ctx, *snap, fmt.Sprintf("web:v%d", i+1))
		}()
	}
	wg.Wait()
	got, _ := db.GetDesiredService(ctx, "web")
	for i := range n {
		if got.Env[fmt.Sprintf("K%d", i)] != "v" {
			t.Fatalf("key K%d lost, env has %d keys", i, len(got.Env))
		}
	}
}
