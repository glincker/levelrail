package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestMigration3039_CIOIDCTrustKeyIncludesProviderAndIssuer(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	old, err := Open(ctx, path, WithMigrationCeiling(3038))
	if err != nil {
		t.Fatalf("open at 3038: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO ci_oidc_trusts (id, service_name, repository, ref_pattern, environment, policy_id, created_by, created_at) VALUES ('cit_gh', 'web', 'acme/web', 'refs/heads/main', '', 'pol_1', 'admin', '2026-10-01T00:00:00Z')`,
		`INSERT INTO ci_oidc_trusts (id, service_name, repository, ref_pattern, environment, policy_id, created_by, created_at, provider, issuer, protected_ref_only) VALUES ('cit_gl', 'web', 'acme/web', 'refs/tags/v*', 'production', 'pol_2', 'admin', '2026-10-02T00:00:00Z', 'gitlab', 'https://gitlab.com', 1)`,
	} {
		if _, err := old.ExecContext(ctx, q); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	cur, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer func() { _ = cur.Close() }()

	got, err := cur.ListCIOIDCTrusts(ctx, "web")
	if err != nil || len(got) != 2 {
		t.Fatalf("rows after rebuild = %d, err = %v", len(got), err)
	}
	if got[0].ID != "cit_gh" || got[0].Provider != "github" || got[1].Issuer != "https://gitlab.com" || !got[1].ProtectedRefOnly || got[1].Environment != "production" {
		t.Fatalf("rows not preserved: %+v", got)
	}
	var idx int
	if err := cur.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'ci_oidc_trusts_repo'`).Scan(&idx); err != nil || idx != 1 {
		t.Fatalf("repository index after rebuild = %d, err = %v", idx, err)
	}

	trust := func(id, provider, issuer string) CIOIDCTrust {
		return CIOIDCTrust{ID: id, ServiceName: "web", Repository: "acme/web", RefPattern: "refs/heads/main", PolicyID: "pol_" + id, CreatedAt: time.Now(), Provider: provider, Issuer: issuer}
	}
	if err := cur.SaveCIOIDCTrust(ctx, trust("cit_a", "gitlab", "https://gitlab.com")); err != nil {
		t.Fatalf("gitlab trust beside a github trust on the same path: %v", err)
	}
	if err := cur.SaveCIOIDCTrust(ctx, trust("cit_b", "gitlab", "https://gitlab.example.com")); err != nil {
		t.Fatalf("second gitlab issuer on the same path: %v", err)
	}
	if err := cur.SaveCIOIDCTrust(ctx, trust("cit_c", "gitlab", "https://gitlab.com")); !errors.Is(err, ErrCIOIDCTrustExists) {
		t.Fatalf("exact duplicate: err = %v, want ErrCIOIDCTrustExists", err)
	}
}
