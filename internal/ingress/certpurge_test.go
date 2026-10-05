package ingress

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPurgeCertsFromOtherIssuers(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seed := []string{
		"certificates/acme-staging-v02.api.letsencrypt.org-directory/a.example/a.example.crt",
		"certificates/acme-staging-v02.api.letsencrypt.org-directory/a.example/a.example.key",
		"certificates/local/b.example/b.example.crt",
		"certificates/acme-v02.api.letsencrypt.org-directory/c.example/c.example.crt",
		"acme/acme-v02.api.letsencrypt.org-directory/users/x/x.json",
	}
	for _, k := range seed {
		if err := db.SaveCertStorageValue(ctx, k, []byte("v")); err != nil {
			t.Fatal(err)
		}
	}

	n, err := PurgeCertsFromOtherIssuers(ctx, db, "")
	if err != nil || n != 3 {
		t.Fatalf("purge for production = %d, %v; want 3 removed", n, err)
	}
	for _, k := range []string{seed[3], seed[4]} {
		if ok, _ := db.ExistsCertStorageValue(ctx, k); !ok {
			t.Errorf("%s was removed but belongs to the kept issuer or is not a certificate", k)
		}
	}
	if n, err = PurgeCertsFromOtherIssuers(ctx, db, "https://acme-v02.api.letsencrypt.org/directory"); err != nil || n != 0 {
		t.Fatalf("second purge = %d, %v; want idempotent 0", n, err)
	}
}
