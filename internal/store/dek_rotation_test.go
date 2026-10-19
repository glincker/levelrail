package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedDEKService(t *testing.T, db *DB, name string) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveServiceDEK(ctx, name, []byte("wrapped-old")); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"A", "B"} {
		if err := db.SaveSecretValue(ctx, name, k, []byte("ct-old-"+k)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRotateServiceDEK_SwapsDEKAndValuesAtomically(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedDEKService(t, db, "web")

	at := time.Now().UTC()
	n, err := db.RotateServiceDEK(ctx, "web", at, func(w []byte, rows []SecretCiphertext) ([]byte, [][]byte, error) {
		if string(w) != "wrapped-old" || len(rows) != 2 {
			t.Fatalf("pre-image = %q, %d rows", w, len(rows))
		}
		return []byte("wrapped-new"), [][]byte{[]byte("ct-new-A"), []byte("ct-new-B")}, nil
	})
	if err != nil || n != 2 {
		t.Fatalf("RotateServiceDEK() = %d, %v", n, err)
	}
	if got, _ := db.GetServiceDEK(ctx, "web"); string(got) != "wrapped-new" {
		t.Fatalf("DEK = %q", got)
	}
	if got, _ := db.GetSecretValue(ctx, "web", "B"); string(got) != "ct-new-B" {
		t.Fatalf("value = %q", got)
	}
	st, err := db.ListServiceDEKStatus(ctx)
	if err != nil || len(st) != 1 || st[0].RotatedAt.IsZero() || st[0].ValueCount != 2 {
		t.Fatalf("status = %+v, %v", st, err)
	}
}

func TestRotateServiceDEK_RewriteErrorRollsBack(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedDEKService(t, db, "web")
	boom := errors.New("boom")
	if _, err := db.RotateServiceDEK(ctx, "web", time.Now(), func([]byte, []SecretCiphertext) ([]byte, [][]byte, error) {
		return nil, nil, boom
	}); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if got, _ := db.GetServiceDEK(ctx, "web"); string(got) != "wrapped-old" {
		t.Fatalf("DEK changed to %q", got)
	}
}

func TestRotateServiceDEK_MissingService(t *testing.T) {
	db := openTestDB(t)
	_, err := db.RotateServiceDEK(context.Background(), "ghost", time.Now(), func([]byte, []SecretCiphertext) ([]byte, [][]byte, error) {
		return nil, nil, nil
	})
	if !errors.Is(err, ErrServiceDEKNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestSecretKeyPolicyAndRuns(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	p, err := db.GetSecretKeyPolicy(ctx)
	if err != nil || p.MaxKeyAgeDays != 0 || p.RemindEveryDays != DefaultRemindEveryDays {
		t.Fatalf("default policy = %+v, %v", p, err)
	}
	now := time.Now().UTC()
	if err := db.SaveSecretKeyPolicy(ctx, SecretKeyPolicy{MaxKeyAgeDays: 90, RemindEveryDays: 3, UpdatedBy: "root"}, now); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkSecretKeyReminded(ctx, now); err != nil {
		t.Fatal(err)
	}
	p, _ = db.GetSecretKeyPolicy(ctx)
	if p.MaxKeyAgeDays != 90 || p.RemindEveryDays != 3 || p.LastRemindedAt.IsZero() {
		t.Fatalf("policy = %+v", p)
	}

	if _, ok, _ := db.LatestDEKRotationRun(ctx); ok {
		t.Fatal("no run expected")
	}
	run := DEKRotationRun{ID: "dek_1", StartedAt: now, Total: 4, Actor: "root"}
	if err := db.CreateDEKRotationRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	run.Done, run.FinishedAt = 4, now
	if err := db.UpdateDEKRotationRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.LatestDEKRotationRun(ctx)
	if err != nil || !ok || !got.Finished() || got.Done != 4 {
		t.Fatalf("run = %+v, %v, %v", got, ok, err)
	}
}
