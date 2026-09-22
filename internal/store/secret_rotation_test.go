package store

import (
	"context"
	"testing"
	"time"
)

// backdateSecretValueUpdatedAt rewrites service_secret_values.updated_at
// for (serviceName, envKey) directly, bypassing SaveSecretValue's own
// "always now" upsert: the only way a test can simulate a secret that
// has genuinely gone stale without sleeping for real days.
func backdateSecretValueUpdatedAt(t *testing.T, db *DB, serviceName, envKey string, at time.Time) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		UPDATE service_secret_values SET updated_at = ? WHERE service_name = ? AND env_key = ?
	`, at.UTC().Format(secretRotationCutoffLayout), serviceName, envKey)
	if err != nil {
		t.Fatalf("backdate service_secret_values.updated_at: %v", err)
	}
}

// backdateProjectEnvVarUpdatedAt is backdateSecretValueUpdatedAt's
// project_env_vars counterpart.
func backdateProjectEnvVarUpdatedAt(t *testing.T, db *DB, projectID, key string, at time.Time) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		UPDATE project_env_vars SET updated_at = ? WHERE project_id = ? AND key = ?
	`, at.UTC().Format(secretRotationCutoffLayout), projectID, key)
	if err != nil {
		t.Fatalf("backdate project_env_vars.updated_at: %v", err)
	}
}

func TestCountStaleSecrets_FreshSecretsAreNotStale(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveServiceDEK(ctx, "web", []byte("dek")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}
	if err := db.SaveSecretValue(ctx, "web", "API_KEY", []byte("ct")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}

	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)
	n, err := db.CountStaleSecrets(ctx, cutoff)
	if err != nil {
		t.Fatalf("CountStaleSecrets() error = %v", err)
	}
	if n != 0 {
		t.Errorf("CountStaleSecrets() = %d, want 0 for a secret just set", n)
	}
}

func TestCountStaleSecrets_CountsOldPerAppSecrets(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveServiceDEK(ctx, "web", []byte("dek")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}
	if err := db.SaveSecretValue(ctx, "web", "API_KEY", []byte("ct1")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}
	if err := db.SaveSecretValue(ctx, "web", "DB_PASSWORD", []byte("ct2")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}
	backdateSecretValueUpdatedAt(t, db, "web", "API_KEY", time.Now().Add(-100*24*time.Hour))

	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)
	n, err := db.CountStaleSecrets(ctx, cutoff)
	if err != nil {
		t.Fatalf("CountStaleSecrets() error = %v", err)
	}
	if n != 1 {
		t.Errorf("CountStaleSecrets() = %d, want 1 (only API_KEY was backdated)", n)
	}
}

func TestCountStaleSecrets_CountsOldSharedSecretEnvVars(t *testing.T) {
	db, ctx := newSeededProjectDB(t)
	if err := db.SetProjectSecretEnvVar(ctx, "proj_test1", "API_KEY"); err != nil {
		t.Fatalf("SetProjectSecretEnvVar() error = %v", err)
	}
	backdateProjectEnvVarUpdatedAt(t, db, "proj_test1", "API_KEY", time.Now().Add(-100*24*time.Hour))

	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)
	n, err := db.CountStaleSecrets(ctx, cutoff)
	if err != nil {
		t.Fatalf("CountStaleSecrets() error = %v", err)
	}
	if n != 1 {
		t.Errorf("CountStaleSecrets() = %d, want 1 for the backdated project-tier secret", n)
	}
}

// TestCountStaleSecrets_IgnoresPlainSharedEnvVars proves the shared-env
// side of the count is scoped to is_secret = 1, the same scope
// ListProjectSecretEnvKeys already uses: an old plain (non-secret) shared
// env var is not a credential and must never inflate this count.
func TestCountStaleSecrets_IgnoresPlainSharedEnvVars(t *testing.T) {
	db, ctx := newSeededProjectDB(t)
	if err := db.SetProjectEnvVars(ctx, "proj_test1", map[string]string{"LOG_LEVEL": "info"}); err != nil {
		t.Fatalf("SetProjectEnvVars() error = %v", err)
	}
	backdateProjectEnvVarUpdatedAt(t, db, "proj_test1", "LOG_LEVEL", time.Now().Add(-200*24*time.Hour))

	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)
	n, err := db.CountStaleSecrets(ctx, cutoff)
	if err != nil {
		t.Fatalf("CountStaleSecrets() error = %v", err)
	}
	if n != 0 {
		t.Errorf("CountStaleSecrets() = %d, want 0: a plain (non-secret) shared env var must never count", n)
	}
}

func TestCountStaleSecrets_NoSecretsAtAll(t *testing.T) {
	db := openTestDB(t)
	n, err := db.CountStaleSecrets(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("CountStaleSecrets() error = %v", err)
	}
	if n != 0 {
		t.Errorf("CountStaleSecrets() on a fresh store = %d, want 0", n)
	}
}
