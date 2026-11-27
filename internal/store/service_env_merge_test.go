package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestMergeServiceEnvConcurrentWritersLoseNothing(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "x:1", Port: 80, Env: map[string]string{"BASE": "1"}, Domains: []string{"a.com"}}); err != nil {
		t.Fatal(err)
	}
	const writers = 30
	var wg sync.WaitGroup
	stop := make(chan struct{})
	// Other writers hit the same row while env merges run; none may drop a merged key.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = db.RestartService(ctx, "web")
				_, _ = db.GetDesiredService(ctx, "web")
			}
		}
	}()
	var mergers sync.WaitGroup
	for i := range writers {
		mergers.Add(1)
		go func() {
			defer mergers.Done()
			key := fmt.Sprintf("K%d", i)
			if _, err := db.MergeServiceEnv(ctx, "web", map[string]string{key: "v"}, nil); err != nil {
				t.Errorf("merge %s: %v", key, err)
			}
		}()
	}
	mergers.Wait()
	close(stop)
	wg.Wait()

	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	for i := range writers {
		if svc.Env[fmt.Sprintf("K%d", i)] != "v" {
			t.Fatalf("key K%d lost: %v", i, svc.Env)
		}
	}
	if svc.Env["BASE"] != "1" || svc.Image != "x:1" || len(svc.Domains) != 1 {
		t.Fatalf("other fields changed: %+v", svc)
	}
}

func TestMergeServiceEnvSetUnsetIdempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "x:1", Port: 80, Env: map[string]string{"A": "1", "B": "2"}}); err != nil {
		t.Fatal(err)
	}
	res, err := db.MergeServiceEnv(ctx, "web", map[string]string{"C": "3", "A": "9"}, []string{"B", "NOPE"})
	if err != nil || !res.Changed || !res.EnvDirty {
		t.Fatalf("first merge = %+v, %v", res, err)
	}
	svc, _ := db.GetDesiredService(ctx, "web")
	if len(svc.Env) != 2 || svc.Env["A"] != "9" || svc.Env["C"] != "3" || !svc.EnvDirty {
		t.Fatalf("stored = %+v", svc)
	}
	// Restart clears the latch; an identical merge must leave it clear and change nothing.
	if err := db.RestartService(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	res, err = db.MergeServiceEnv(ctx, "web", map[string]string{"C": "3", "A": "9"}, []string{"B"})
	if err != nil || res.Changed || res.EnvDirty {
		t.Fatalf("repeat merge = %+v, %v", res, err)
	}
	if svc, _ = db.GetDesiredService(ctx, "web"); svc.EnvDirty {
		t.Fatal("idempotent merge must not set env_dirty")
	}
	if _, err := db.MergeServiceEnv(ctx, "missing", map[string]string{"A": "1"}, nil); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("missing app err = %v", err)
	}
}

func TestMergeServiceEnvRefusesSecretKey(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "x:1", Port: 80, SecretEnv: []SecretEnvRef{{Name: "TOKEN"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MergeServiceEnv(ctx, "web", map[string]string{"TOKEN": "x"}, nil); !errors.Is(err, ErrEnvKeyIsSecret) {
		t.Fatalf("err = %v", err)
	}
}
