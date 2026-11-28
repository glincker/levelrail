package store

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func seedEnvApp(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "x:1", Port: 80, Env: map[string]string{"A": "1"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveServiceDEK(ctx, "web", []byte("wrapped")); err != nil {
		t.Fatal(err)
	}
}

func envVersionOf(t *testing.T, db *DB) int {
	t.Helper()
	v, err := db.EnvVersion(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestEnvVersion_ConcurrentWritersBumpExactlyTwo(t *testing.T) {
	db := openTestDB(t)
	seedEnvApp(t, db)
	ctx := context.Background()
	start := envVersionOf(t, db)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := db.MergeServiceEnv(ctx, "web", map[string]string{"B": "2"}, nil); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := db.SetServiceSecretEnvDeclared(ctx, "web", "TOKEN", true); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()
	if got := envVersionOf(t, db); got != start+2 {
		t.Fatalf("env_version = %d, want %d", got, start+2)
	}
}

func TestApplyEnvPlan_StaleVersionLeavesEnvUnchanged(t *testing.T) {
	db := openTestDB(t)
	seedEnvApp(t, db)
	ctx := context.Background()
	if _, err := db.MergeServiceEnv(ctx, "web", map[string]string{"B": "2"}, nil); err != nil {
		t.Fatal(err)
	}
	stale := 0
	_, err := db.ApplyEnvPlan(ctx, "web", EnvPlan{ExpectedVersion: &stale, Set: map[string]string{"C": "3"}})
	if !errors.Is(err, ErrEnvVersionConflict) {
		t.Fatalf("err = %v, want ErrEnvVersionConflict", err)
	}
	svc, _ := db.GetDesiredService(ctx, "web")
	if _, ok := svc.Env["C"]; ok || len(svc.Env) != 2 {
		t.Fatalf("env changed on a stale plan: %v", svc.Env)
	}
}

func TestApplyEnvPlan_DryRunChangesNothing(t *testing.T) {
	db := openTestDB(t)
	seedEnvApp(t, db)
	ctx := context.Background()
	before := envVersionOf(t, db)
	res, err := db.ApplyEnvPlan(ctx, "web", EnvPlan{DryRun: true, Set: map[string]string{"C": "3"}, SecretSet: map[string][]byte{"TOKEN": []byte("ct")}})
	if err != nil || len(res.Changes) != 2 {
		t.Fatalf("dry run = %+v, %v", res, err)
	}
	svc, _ := db.GetDesiredService(ctx, "web")
	if len(svc.Env) != 1 || len(svc.SecretEnv) != 0 || envVersionOf(t, db) != before {
		t.Fatalf("dry run wrote: %+v", svc)
	}
	if has, _ := db.HasSecretValue(ctx, "web", "TOKEN"); has {
		t.Fatal("dry run saved ciphertext")
	}
}

func TestApplyEnvPlan_FaultMidPlanRollsBackEverything(t *testing.T) {
	db := openTestDB(t)
	seedEnvApp(t, db)
	ctx := context.Background()
	if err := db.SaveSecretValue(ctx, "web", "OLD", []byte("old")); err != nil {
		t.Fatal(err)
	}
	envApplyFault = func(stage string) error {
		if stage == "before-columns" {
			return errors.New("injected")
		}
		return nil
	}
	t.Cleanup(func() { envApplyFault = nil })
	before := envVersionOf(t, db)
	_, err := db.ApplyEnvPlan(ctx, "web", EnvPlan{
		Set: map[string]string{"C": "3"}, SecretSet: map[string][]byte{"TOKEN": []byte("ct")}, SecretDelete: []string{"OLD"},
	})
	if err == nil {
		t.Fatal("expected the injected failure")
	}
	svc, _ := db.GetDesiredService(ctx, "web")
	hasToken, _ := db.HasSecretValue(ctx, "web", "TOKEN")
	hasOld, _ := db.HasSecretValue(ctx, "web", "OLD")
	if len(svc.Env) != 1 || len(svc.SecretEnv) != 0 || hasToken || !hasOld || envVersionOf(t, db) != before {
		t.Fatalf("partial apply survived: env=%v secrets=%v token=%v old=%v", svc.Env, svc.SecretEnv, hasToken, hasOld)
	}
}

func TestApplyEnvPlan_LockedSecretBlocksInsideTx(t *testing.T) {
	db := openTestDB(t)
	seedEnvApp(t, db)
	ctx := context.Background()
	if err := db.SaveSecretValue(ctx, "web", "TOKEN", []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSecretLocked(ctx, "web", "TOKEN", true); err != nil {
		t.Fatal(err)
	}
	_, err := db.ApplyEnvPlan(ctx, "web", EnvPlan{Set: map[string]string{"C": "3"}, SecretSet: map[string][]byte{"TOKEN": []byte("new")}})
	if !errors.Is(err, ErrEnvLockedSecret) {
		t.Fatalf("err = %v, want ErrEnvLockedSecret", err)
	}
	if svc, _ := db.GetDesiredService(ctx, "web"); len(svc.Env) != 1 {
		t.Fatalf("plain change leaked past a locked secret: %v", svc.Env)
	}
	res, err := db.ApplyEnvPlan(ctx, "web", EnvPlan{OverwriteLocked: true, SecretSet: map[string][]byte{"TOKEN": []byte("new")}})
	if err != nil || !res.Changed {
		t.Fatalf("overwrite_locked apply = %+v, %v", res, err)
	}
}

func TestApplyEnvPlan_DualHomeRefused(t *testing.T) {
	db := openTestDB(t)
	seedEnvApp(t, db)
	_, err := db.ApplyEnvPlan(context.Background(), "web", EnvPlan{SecretSet: map[string][]byte{"A": []byte("ct")}})
	if !errors.Is(err, ErrEnvDualHome) {
		t.Fatalf("err = %v, want ErrEnvDualHome", err)
	}
	res, err := db.ApplyEnvPlan(context.Background(), "web", EnvPlan{Convert: []string{"A"}, SecretSet: map[string][]byte{"A": []byte("ct")}})
	if err != nil || !res.Changed {
		t.Fatalf("convert = %+v, %v", res, err)
	}
	svc, _ := db.GetDesiredService(context.Background(), "web")
	if _, ok := svc.Env["A"]; ok || len(svc.SecretEnv) != 1 {
		t.Fatalf("convert left two homes: env=%v secrets=%v", svc.Env, svc.SecretEnv)
	}
}
