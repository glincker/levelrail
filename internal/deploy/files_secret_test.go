package deploy

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fileSecretStore struct {
	ServiceStore
	prev *store.DesiredService
}

func (f *fileSecretStore) GetDesiredService(context.Context, string) (*store.DesiredService, error) {
	return f.prev, nil
}

type fileSecretVault struct {
	SecretChecker
	vals map[string]string
}

func (v *fileSecretVault) SetValue(_ context.Context, _, k, val string) error {
	v.vals[k] = val
	return nil
}

func (v *fileSecretVault) DeleteValue(_ context.Context, _, k string) error {
	delete(v.vals, k)
	return nil
}

func TestPersistFileSecretsDropsRemovedSecretFile(t *testing.T) {
	ctx := context.Background()
	vault := &fileSecretVault{vals: map[string]string{}}
	st := &fileSecretStore{}
	p := &Pipeline{store: st, secrets: vault}
	key := store.FileSecretKey("/etc/app/key.pem")

	with := store.DesiredService{Name: "web", Files: []store.ServiceFile{{Path: "/etc/app/key.pem", Secret: true, Content: "s3"}}}
	if err := p.persistFileSecrets(ctx, &with); err != nil {
		t.Fatal(err)
	}
	if vault.vals[key] != "s3" || with.Files[0].Content != "" {
		t.Fatalf("secret not stored: %v", vault.vals)
	}
	st.prev = &with

	without := store.DesiredService{Name: "web"}
	if err := p.persistFileSecrets(ctx, &without); err != nil {
		t.Fatal(err)
	}
	if _, ok := vault.vals[key]; ok {
		t.Fatal("orphaned secret file value survived")
	}
	st.prev = &without

	// Reusing the path with no content must not bring the old value back.
	reuse := store.DesiredService{Name: "web", Files: []store.ServiceFile{{Path: "/etc/app/key.pem", Secret: true}}}
	if err := p.persistFileSecrets(ctx, &reuse); err != nil {
		t.Fatal(err)
	}
	if _, ok := vault.vals[key]; ok {
		t.Fatal("value restored without being re-supplied")
	}
}
