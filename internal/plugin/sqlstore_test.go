package plugin

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestSQLStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := NewSQLStore(db)

	rec := Record{
		Name: "example", Version: "1.0.0", Kind: KindNotificationChannel, Publisher: "Example Org", Image: "r/e@" + testDigest, Digest: testDigest,
		Manifest: "m", Signature: "sig", KeyID: "k", RequestedAbilities: []string{"read"}, ApprovedAbilities: []string{}, OutboundHosts: []string{"a.example"},
		ConfigJSON: "{}", State: StateInstalled, InstalledAt: "t", InstalledBy: "admin", UpdatedAt: "t",
	}
	if err := s.InsertPlugin(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertPlugin(ctx, rec); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate insert = %v", err)
	}
	rec.State, rec.ApprovedAbilities = StateEnabled, []string{"read"}
	if err := s.UpdatePlugin(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLastError(ctx, "example", "boom", "t2"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetPlugin(ctx, "example")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateEnabled || !reflect.DeepEqual(got.ApprovedAbilities, []string{"read"}) || got.LastError != "boom" || !reflect.DeepEqual(got.OutboundHosts, []string{"a.example"}) {
		t.Fatalf("round trip = %+v", got)
	}
	if err := s.UpdatePlugin(ctx, Record{Name: "nope"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update of a missing plugin = %v", err)
	}
	if err := s.DeletePlugin(ctx, "example"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPlugin(ctx, "example"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete = %v", err)
	}

	k := TrustedKey{KeyID: "ed25519:aa", Name: "pub", PublicKey: "pk", AddedAt: "t", AddedBy: "admin"}
	if err := s.AddKey(ctx, k); err != nil {
		t.Fatal(err)
	}
	if keys, _ := s.ListKeys(ctx); len(keys) != 1 || keys[0] != k {
		t.Fatalf("keys = %+v", keys)
	}
	if err := s.RemoveKey(ctx, k.KeyID); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveKey(ctx, k.KeyID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second remove = %v", err)
	}
}

func TestConfigFromEnv(t *testing.T) {
	env := map[string]string{"APP_PLUGIN_MEMORY_MB": "256", "APP_PLUGIN_CALL_TIMEOUT": "3s", "APP_PLUGIN_MAX_PAYLOAD_KB": "8", "APP_PLUGIN_CPUS": "bogus"}
	c := ConfigFromEnv("ns", "/d", func(k string) (string, bool) { v, ok := env[k]; return v, ok }).withDefaults()
	if c.MemoryBytes != 256<<20 || c.CallTimeout.Seconds() != 3 || c.MaxEventBytes != 8<<10 {
		t.Fatalf("config = %+v", c)
	}
	if c.NanoCPUs != 500_000_000 {
		t.Fatalf("a bad value should keep the default, got %d", c.NanoCPUs)
	}
}
