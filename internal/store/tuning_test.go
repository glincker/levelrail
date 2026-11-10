package store

import (
	"context"
	"testing"
)

func TestTuning_AppliesToWriterAndReaders(t *testing.T) {
	db := openTestDB(t)
	if err := db.ApplyTuning(context.Background(), Tuning{CacheSizeMB: 8, TempStore: "memory", Synchronous: "normal"}); err != nil {
		t.Fatal(err)
	}
	var cache, temp, sync int
	if err := db.QueryRow(`PRAGMA cache_size`).Scan(&cache); err != nil || cache != -8*1024 {
		t.Fatalf("writer cache_size %d err %v", cache, err)
	}
	if err := db.QueryRow(`PRAGMA temp_store`).Scan(&temp); err != nil || temp != 2 {
		t.Fatalf("temp_store %d err %v", temp, err)
	}
	if err := db.QueryRow(`PRAGMA synchronous`).Scan(&sync); err != nil || sync != 1 {
		t.Fatalf("synchronous %d err %v", sync, err)
	}
	if db.ro != nil {
		if err := db.ro.QueryRow(`PRAGMA cache_size`).Scan(&cache); err != nil || cache != -8*1024 {
			t.Fatalf("reader cache_size %d err %v", cache, err)
		}
	}
}

func TestTuning_ZeroValuesChangeNothing(t *testing.T) {
	if got := (Tuning{}).pragmas(); len(got) != 0 {
		t.Fatalf("pragmas %v", got)
	}
}
