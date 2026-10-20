package store

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
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

// TestCheckMigrationVersionsScript_RealEmbeddedFiles shells out to the same
// script CI and the git hooks run, so the go test suite backstops the
// collision check too, not just checkNoDuplicateVersions above.
func TestCheckMigrationVersionsScript_RealEmbeddedFiles(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	script := filepath.Join(repoRoot, "scripts", "check-migration-versions.sh")

	cmd := exec.Command(script) //nolint:gosec // script path built from runtime.Caller(0), not external input
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("check-migration-versions.sh failed: %v\n%s", err, out)
	}
}

func TestMigrationsCurrent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.MigrationsCurrent(ctx); err != nil {
		t.Fatalf("fresh db: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = (SELECT MAX(version) FROM schema_migrations)`); err != nil {
		t.Fatal(err)
	}
	if err := db.MigrationsCurrent(ctx); err == nil {
		t.Fatal("want error with latest migration unapplied")
	}
}

func TestPendingMigrations(t *testing.T) {
	all := []migration{{version: 1, name: "a"}, {version: 2, name: "b"}, {version: 3, name: "c"}, {version: 5, name: "e"}}
	tests := []struct {
		name    string
		applied map[int]bool
		want    []int
	}{
		{"fresh database", map[int]bool{}, []int{1, 2, 3, 5}},
		{"fully applied", map[int]bool{1: true, 2: true, 3: true, 5: true}, nil},
		{"only newer ones", map[int]bool{1: true, 2: true}, []int{3, 5}},
		{"older migration merged after a newer one ran", map[int]bool{1: true, 3: true, 5: true}, []int{2}},
		{"unknown applied version is ignored", map[int]bool{1: true, 2: true, 3: true, 5: true, 9: true}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []int
			for _, m := range pendingMigrations(all, tt.applied) {
				got = append(got, m.version)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("pending = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("pending = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
