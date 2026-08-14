package store

import (
	"testing"
)

func TestParseMigrationFilename(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		wantVersion int
		wantName    string
		wantErr     bool
	}{
		{
			name:        "valid",
			filename:    "0001_reconcile_status.sql",
			wantVersion: 1,
			wantName:    "reconcile_status",
		},
		{
			name:        "valid, multi-word description with underscores preserved",
			filename:    "0042_add_application_settings.sql",
			wantVersion: 42,
			wantName:    "add_application_settings",
		},
		{
			name:     "missing underscore separator",
			filename: "0001.sql",
			wantErr:  true,
		},
		{
			name:     "non-numeric version prefix",
			filename: "abcd_reconcile_status.sql",
			wantErr:  true,
		},
		{
			name:     "zero version rejected",
			filename: "0000_reconcile_status.sql",
			wantErr:  true,
		},
		{
			name:     "negative version rejected",
			filename: "-001_reconcile_status.sql",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVersion, gotName, err := parseMigrationFilename(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseMigrationFilename(%q) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("version = %d, want %d", gotVersion, tt.wantVersion)
			}
			if gotName != tt.wantName {
				t.Errorf("name = %q, want %q", gotName, tt.wantName)
			}
		})
	}
}

func TestCheckNoDuplicateVersions(t *testing.T) {
	tests := []struct {
		name       string
		migrations []migration
		wantErr    bool
	}{
		{
			name: "no duplicates",
			migrations: []migration{
				{version: 1, name: "a"},
				{version: 2, name: "b"},
			},
		},
		{
			name: "duplicate version",
			migrations: []migration{
				{version: 1, name: "a"},
				{version: 1, name: "b"},
			},
			wantErr: true,
		},
		{
			name:       "empty",
			migrations: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkNoDuplicateVersions(tt.migrations)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkNoDuplicateVersions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadMigrations_RealEmbeddedFiles(t *testing.T) {
	// Not a fake: this reads the actual migrations/*.sql embedded in this
	// binary, so a malformed filename or duplicate version in a real
	// migration file fails the test suite immediately rather than only
	// at runtime against a real database.
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations() error = %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("expected at least one embedded migration, got none")
	}
	for i := 1; i < len(migrations); i++ {
		if migrations[i-1].version >= migrations[i].version {
			t.Errorf("migrations not sorted ascending by version: %d then %d", migrations[i-1].version, migrations[i].version)
		}
	}
	for _, m := range migrations {
		if m.sql == "" {
			t.Errorf("migration %04d_%s has empty SQL content", m.version, m.name)
		}
	}
}
