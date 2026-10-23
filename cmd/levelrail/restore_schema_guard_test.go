package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestSchemaBehindBinary(t *testing.T) {
	newest, err := store.MaxSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		version    int
		create     bool
		wantBehind bool
	}{
		{"restored older schema is left alone", newest - 1, true, true},
		{"current schema is recorded as before", newest, true, false},
		{"missing database keeps the old behavior", 0, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			live := filepath.Join(t.TempDir(), "levelrail.db")
			if tt.create {
				db, err := sql.Open("sqlite", "file:"+live)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = db.Close() }()
				if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT, applied_at TEXT)`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, 'x', '')`, tt.version); err != nil {
					t.Fatal(err)
				}
			}
			_, _, behind := schemaBehindBinary(context.Background(), live)
			if behind != tt.wantBehind {
				t.Fatalf("behind = %v, want %v", behind, tt.wantBehind)
			}
		})
	}
}
