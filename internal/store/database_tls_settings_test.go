package store

import (
	"context"
	"testing"
)

func TestDatabaseTLSMode(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	mode, err := db.GetDatabaseTLSMode(ctx, "main")
	if err != nil || mode != DatabaseTLSModeDefault {
		t.Fatalf("unset mode = %q, %v; want default", mode, err)
	}
	if err := db.SetDatabaseTLSMode(ctx, "main", DatabaseTLSModeOff); err != nil {
		t.Fatalf("SetDatabaseTLSMode: %v", err)
	}
	if mode, _ = db.GetDatabaseTLSMode(ctx, "main"); mode != DatabaseTLSModeOff {
		t.Fatalf("mode = %q, want off", mode)
	}
	if err := db.SetDatabaseTLSMode(ctx, "main", "bogus"); err == nil {
		t.Fatal("expected an error for an unknown mode")
	}
}

func TestDatabaseTLSEffective(t *testing.T) {
	tests := []struct {
		engine, mode string
		want         bool
	}{
		{EnginePostgres, DatabaseTLSModeDefault, true},
		{EngineRedis, DatabaseTLSModeDefault, true},
		{EngineMySQL, DatabaseTLSModeDefault, false},
		{EngineMySQL, DatabaseTLSModeOn, true},
		{EngineMariaDB, DatabaseTLSModeOn, true},
		{EngineRedis, DatabaseTLSModeOff, false},
	}
	for _, tc := range tests {
		if got := DatabaseTLSEffective(tc.engine, tc.mode); got != tc.want {
			t.Errorf("DatabaseTLSEffective(%s, %s) = %v, want %v", tc.engine, tc.mode, got, tc.want)
		}
	}
}

func TestDatabaseTLSOnAtCreate(t *testing.T) {
	for engine, want := range map[string]bool{
		EngineMySQL: true, EngineMariaDB: true, EngineMongoDB: true,
		EnginePostgres: false, EngineRedis: false, EngineClickHouse: false,
	} {
		if got := DatabaseTLSOnAtCreate(engine); got != want {
			t.Errorf("DatabaseTLSOnAtCreate(%q) = %v, want %v", engine, got, want)
		}
	}
}
