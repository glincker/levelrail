package database

import (
	"strings"
	"testing"
)

func TestImageRef_PgvectorVariant(t *testing.T) {
	tests := []struct {
		name, engine, version, want string
	}{
		{"plain postgres", "postgres", "16", "postgres:16"},
		{"plain alpine tag", "postgres", "17-alpine", "postgres:17-alpine"},
		{"empty is latest", "postgres", "", "postgres:latest"},
		{"latest", "postgres", "latest", "postgres:latest"},
		{"pgvector 17", "postgres", "17-pgvector", "pgvector/pgvector:pg17"},
		{"pgvector 16", "postgres", "16-pgvector", "pgvector/pgvector:pg16"},
		{"redis untouched", "redis", "7", "redis:7"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ImageRef(tc.engine, tc.version); got != tc.want {
				t.Fatalf("ImageRef(%q, %q) = %q, want %q", tc.engine, tc.version, got, tc.want)
			}
		})
	}
}

func TestImageRef_PlainNeverGetsPgvectorImage(t *testing.T) {
	for _, v := range []string{"", "latest", "13", "16", "16.4", "17", "17-alpine", "18", "17-bookworm"} {
		if got := ImageRef("postgres", v); strings.Contains(got, "pgvector") {
			t.Fatalf("plain version %q mapped to %q", v, got)
		}
	}
}

func TestParsePgvectorVersion(t *testing.T) {
	tests := []struct {
		version   string
		major     int
		isVariant bool
		wantErr   bool
	}{
		{"17-pgvector", 17, true, false},
		{"13-pgvector", 13, true, false},
		{"16", 0, false, false},
		{"", 0, false, false},
		{"latest", 0, false, false},
		{"17.4-pgvector", 0, true, true},
		{"pgvector", 0, true, true},
		{"-pgvector", 0, true, true},
		{"17-pgvector-x", 0, true, true},
		{"12-pgvector", 0, true, true},
		{"7-pgvector", 0, true, true},
		{"17-PGVECTOR", 0, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.version, func(t *testing.T) {
			major, isVariant, err := ParsePgvectorVersion(tc.version)
			if (err != nil) != tc.wantErr || isVariant != tc.isVariant || major != tc.major {
				t.Fatalf("ParsePgvectorVersion(%q) = %d, %v, %v", tc.version, major, isVariant, err)
			}
		})
	}
}

func TestValidateEngineVersion(t *testing.T) {
	if err := ValidateEngineVersion("postgres", "17-pgvector"); err != nil {
		t.Fatalf("valid variant rejected: %v", err)
	}
	if err := ValidateEngineVersion("mysql", "8-pgvector"); err == nil {
		t.Fatal("variant on a non-postgres engine accepted")
	}
	if err := ValidateEngineVersion("postgres", "17.4-pgvector"); err == nil {
		t.Fatal("malformed variant accepted")
	}
	if err := ValidateEngineVersion("redis", "7"); err != nil {
		t.Fatalf("plain version rejected: %v", err)
	}
}

func TestPostgresNeedsPGDATA_Variant(t *testing.T) {
	if !PostgresNeedsPGDATA("18-pgvector") || PostgresNeedsPGDATA("17-pgvector") {
		t.Fatal("PGDATA pin must follow the major of a pgvector version")
	}
}
