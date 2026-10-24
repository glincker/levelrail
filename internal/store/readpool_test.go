package store

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func openFileDB(t *testing.T) *DB {
	t.Helper()
	return openTestDB(t)
}

func TestReadPool_RejectsWrites(t *testing.T) {
	db := openFileDB(t)
	if db.ro == nil {
		t.Fatal("expected a read pool for a file database")
	}
	_, err := db.ro.ExecContext(context.Background(),
		`INSERT INTO apps (id, name, created_at, updated_at) VALUES ('x','x','t','t')`)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "readonly") &&
		!strings.Contains(strings.ToLower(err.Error()), "read-only") &&
		!strings.Contains(strings.ToLower(err.Error()), "read only") {
		t.Fatalf("write on read pool should be rejected, got %v", err)
	}
}

func TestReadPool_SeesCommittedWriteImmediately(t *testing.T) {
	db := openFileDB(t)
	ctx := context.Background()
	for i := range 25 {
		app := App{ID: fmt.Sprintf("app_%d", i), Name: fmt.Sprintf("a%02d", i), CreatedAt: "t", UpdatedAt: "t"}
		if err := db.SaveApp(ctx, app); err != nil {
			t.Fatalf("SaveApp: %v", err)
		}
		if got := db.CountApps(ctx); got != i+1 {
			t.Fatalf("after write %d read pool saw %d apps", i+1, got)
		}
	}
}

func TestReadPool_ConcurrentReadsWithWrites(t *testing.T) {
	db := openFileDB(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				if _, err := db.ListApps(ctx); err != nil {
					t.Errorf("ListApps: %v", err)
					return
				}
			}
		}()
	}
	for i := range 50 {
		if err := db.SaveApp(ctx, App{ID: fmt.Sprintf("w_%d", i), Name: fmt.Sprintf("w%02d", i), CreatedAt: "t", UpdatedAt: "t"}); err != nil {
			t.Fatalf("SaveApp: %v", err)
		}
	}
	wg.Wait()
}

func TestReadPool_DisabledAndMemoryFallBack(t *testing.T) {
	t.Setenv(EnvReadConns, "0")
	db := openFileDB(t)
	if db.ro != nil {
		t.Fatal("APP_DB_READ_CONNS=0 should disable the pool")
	}
	t.Setenv(EnvReadConns, "4")
	if p := openReadPool(context.Background(), ":memory:"); p != nil {
		t.Fatal("in-memory databases must not get a read pool")
	}
}

func TestReadPool_CloseClosesBoth(t *testing.T) {
	db := openTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := db.ro.PingContext(context.Background()); err == nil {
		t.Fatal("read pool should be closed")
	}
}

func TestMetrics_ReadPoolFields(t *testing.T) {
	tests := []struct {
		name        string
		dropPool    bool
		wantEnabled bool
		wantSize    int
	}{
		{"pool nil", true, false, 0},
		{"pool enabled", false, true, readPoolSize()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := openFileDB(t)
			if tc.dropPool {
				db.ro = nil
			}
			m := db.Metrics()
			if m.ReadPoolEnabled != tc.wantEnabled || m.ReadPoolSize != tc.wantSize {
				t.Fatalf("enabled=%v size=%d, want %v %d", m.ReadPoolEnabled, m.ReadPoolSize, tc.wantEnabled, tc.wantSize)
			}
			if !tc.wantEnabled && (m.ReadInUse != 0 || m.ReadWaitCount != 0 || m.ReadWaitMS != 0) {
				t.Fatalf("disabled pool reported usage: %+v", m)
			}
		})
	}
}
