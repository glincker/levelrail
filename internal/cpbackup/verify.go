package cpbackup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Check is one step of a backup verification.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// VerifyResult is the outcome of verifying one backup. Nothing is restored.
type VerifyResult struct {
	Name       string    `json:"name"`
	OK         bool      `json:"ok"`
	Checks     []Check   `json:"checks"`
	VerifiedAt time.Time `json:"verified_at"`
}

type verification struct {
	OK         bool      `json:"ok"`
	VerifiedAt time.Time `json:"verified_at"`
}

func (m *Manager) readVerification(name string) (verification, bool) {
	var v verification
	data, err := os.ReadFile(filepath.Join(m.dir, name+verifiedSuffix)) //nolint:gosec // name validated against namePattern
	if err != nil || json.Unmarshal(data, &v) != nil {
		return v, false
	}
	return v, true
}

// Verify re-hashes the backup against its recorded checksum, runs SQLite's
// integrity_check, and confirms its schema is not newer than this binary. It
// records the outcome next to the snapshot.
func (m *Manager) Verify(ctx context.Context, name string) (VerifyResult, error) {
	if !ValidName(name) {
		return VerifyResult{}, ErrInvalidName
	}
	path := filepath.Join(m.dir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return VerifyResult{}, ErrNotFound
	} else if err != nil {
		return VerifyResult{}, fmt.Errorf("stat backup: %w", err)
	}

	res := VerifyResult{Name: name, VerifiedAt: m.now().UTC().Truncate(time.Second)}
	version, integrity := verifyIntegrity(ctx, path)
	res.Checks = []Check{checksumCheck(path), integrity, schemaCheck(version, integrity.OK)}
	res.OK = true
	for _, c := range res.Checks {
		res.OK = res.OK && c.OK
	}

	data, err := json.Marshal(verification{OK: res.OK, VerifiedAt: res.VerifiedAt})
	if err == nil {
		err = os.WriteFile(path+verifiedSuffix, data, 0o600)
	}
	if err != nil {
		return VerifyResult{}, fmt.Errorf("record verification: %w", err)
	}
	return res, nil
}

func checksumCheck(path string) Check {
	c := Check{Name: "checksum"}
	_, sum, err := store.FileSHA256(path)
	if err != nil {
		c.Detail = err.Error()
		return c
	}
	recorded, err := os.ReadFile(path + checksumSuffix) //nolint:gosec // path built from a validated name
	if err != nil {
		c.OK = true
		c.Detail = "no recorded checksum (taken before checksums were recorded), computed " + sum
		return c
	}
	want := strings.TrimSpace(string(recorded))
	if want != sum {
		c.Detail = fmt.Sprintf("sha256 mismatch: recorded %s, file is %s", want, sum)
		return c
	}
	c.OK = true
	c.Detail = "sha256 matches the recorded " + sum
	return c
}

func verifyIntegrity(ctx context.Context, path string) (int, Check) {
	c := Check{Name: "integrity"}
	version, err := store.InspectSnapshot(ctx, path)
	if err != nil {
		c.Detail = err.Error()
		return 0, c
	}
	c.OK = true
	c.Detail = "integrity_check ok"
	return version, c
}

func schemaCheck(version int, readable bool) Check {
	c := Check{Name: "schema_version"}
	if !readable {
		c.Detail = "skipped, database could not be read"
		return c
	}
	latest, err := store.MaxSchemaVersion()
	if err != nil {
		c.Detail = err.Error()
		return c
	}
	if version > latest {
		c.Detail = fmt.Sprintf("schema version %d is newer than this binary's %d", version, latest)
		return c
	}
	c.OK = true
	c.Detail = fmt.Sprintf("schema version %d, this binary supports up to %d", version, latest)
	return c
}
