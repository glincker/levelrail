package store

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestMigration3287_ExistingProviderSecretBecomesDefaultAccount(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	old, err := Open(ctx, path, WithMigrationCeiling(3286))
	if err != nil {
		t.Fatalf("open at 3286: %v", err)
	}
	sealed := []byte{1, 2, 3, 4}
	for _, q := range []string{
		`INSERT INTO service_secrets (service_name, wrapped_dek) VALUES ('node-provider/hetzner', x'00'), ('node-provider/oracle', x'00'), ('registry/other', x'00')`,
		`INSERT INTO service_secret_values (service_name, env_key, ciphertext) VALUES ('node-provider/hetzner', 'token', x'01020304'), ('node-provider/oracle', 'token', x'01020304'), ('registry/other', 'token', x'01020304')`,
		`INSERT INTO node_provisions (id, provider, region, size, name, created_at, updated_at) VALUES ('p1', 'hetzner', 'fsn1', 'cx22', 'n1', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
	} {
		if _, err := old.ExecContext(ctx, q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer func() { _ = db.Close() }()

	accounts, err := db.ListCloudAccounts(ctx, "")
	if err != nil || len(accounts) != 2 {
		t.Fatalf("accounts = %d, err %v, want 2 (non node-provider secrets are ignored)", len(accounts), err)
	}
	a, err := db.GetCloudAccount(ctx, DefaultCloudAccountID("hetzner"))
	if err != nil {
		t.Fatalf("default account: %v", err)
	}
	if a.Provider != "hetzner" || a.CredentialRef != NodeProviderSecretsKey("hetzner") || a.Status != CloudAccountStatusActive || string(a.Config) != "{}" {
		t.Errorf("default account = %+v", a)
	}

	var ct []byte
	if err := db.QueryRowContext(ctx, `SELECT ciphertext FROM service_secret_values WHERE service_name = 'node-provider/hetzner'`).Scan(&ct); err != nil || !bytes.Equal(ct, sealed) {
		t.Errorf("secret re-sealed or lost: %v %v", ct, err)
	}
	p, err := db.GetNodeProvision(ctx, "p1")
	if err != nil || p.AccountID != "hetzner-default" || p.CreatedBy != "" || p.TeamID != "" {
		t.Errorf("provision = %+v err %v", p, err)
	}
}

func TestCloudGrantsCascadeAndValidate(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := db.CreateCloudAccount(ctx, CloudAccount{ID: "aws-prod", Provider: "aws", Config: []byte(`{"regions":["eu-west-1"]}`)}); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(time.Hour)
	g := CloudGrant{ID: "g1", AccountID: "aws-prod", ResourceKind: CloudResourceAccount, PrincipalType: CloudPrincipalUser, PrincipalID: "u1",
		Actions: []string{CloudActionView}, GrantedBy: "root:u0", ExpiresAt: &exp}
	if err := db.UpsertCloudGrant(ctx, g); err != nil {
		t.Fatal(err)
	}
	g.Actions = []string{CloudActionDeleteForTest}
	if err := db.UpsertCloudGrant(ctx, g); err == nil {
		t.Error("delete must not be grantable")
	}
	got, err := db.ListCloudGrantsForPrincipals(ctx, "aws-prod", []CloudPrincipal{{CloudPrincipalUser, "u1"}})
	if err != nil || len(got) != 1 || got[0].ExpiresAt == nil {
		t.Fatalf("grants = %+v err %v", got, err)
	}
	if err := db.DeleteCloudAccount(ctx, "aws-prod"); err != nil {
		t.Fatal(err)
	}
	if left, _ := db.ListCloudGrants(ctx, "aws-prod"); len(left) != 0 {
		t.Errorf("grants survived account delete: %+v", left)
	}
}

// CloudActionDeleteForTest is an action the store must refuse to grant.
const CloudActionDeleteForTest = "delete"
