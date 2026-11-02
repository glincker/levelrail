package store

import (
	"context"
	"sync"
	"testing"
)

func TestPatchServiceEnvKeepsOtherFields(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "x:1", Port: 80, Env: map[string]string{"A": "1", "B": "2"}, Domains: []string{"a.com"}}); err != nil {
		t.Fatal(err)
	}
	applied, err := db.PatchServiceEnv(ctx, "web", map[string]string{"B": "2", "A": "stale"}, map[string]string{"B": "3", "A": "9"})
	if err != nil || len(applied) != 1 || applied[0] != "B" {
		t.Fatalf("applied = %v, err %v", applied, err)
	}
	svc, _ := db.GetDesiredService(ctx, "web")
	if svc.Env["A"] != "1" || svc.Env["B"] != "3" || len(svc.Domains) != 1 || svc.Image != "x:1" {
		t.Fatalf("stored = %+v", svc)
	}
	if _, err := db.PatchServiceEnv(ctx, "missing", nil, nil); err == nil {
		t.Fatal("missing service must error")
	}
}

func TestPatchServiceEnvParallelPatchesBothSurvive(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "x:1", Port: 80, Env: map[string]string{"A": "1", "B": "1"}}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, k := range []string{"A", "B"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := db.PatchServiceEnv(ctx, "web", map[string]string{k: "1"}, map[string]string{k: "2"}); err != nil {
				t.Errorf("patch %s: %v", k, err)
			}
		}()
	}
	wg.Wait()
	svc, _ := db.GetDesiredService(ctx, "web")
	if svc.Env["A"] != "2" || svc.Env["B"] != "2" {
		t.Fatalf("env = %+v, both patches must survive", svc.Env)
	}
}
