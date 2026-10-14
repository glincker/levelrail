package cpbackup

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRestore_CarriesSealedNotificationChannelSecrets(t *testing.T) {
	ctx := context.Background()
	e := newDREnv(t)
	e.configure(nil)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	const url = "https://hooks.slack.com/services/T0/B0/backedup"
	key := alerting.NotificationChannelSecretsKey("chn_1")
	if err := secrets.NewManager(e.db, mk).SetValue(ctx, key, "notify_url", url); err != nil {
		t.Fatal(err)
	}
	m := e.backup()

	live := filepath.Join(t.TempDir(), "levelrail.db")
	if _, err := restoreFrom(t, e, m.Key, live, nil); err != nil {
		t.Fatalf("restore: %v", err)
	}
	restored, err := store.Open(ctx, live)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restored.Close() })
	got, err := secrets.NewManager(restored, mk).Resolve(ctx, key, "notify_url")
	if err != nil || got != url {
		t.Fatalf("restored channel secret = %q, %v; want %q", got, err, url)
	}
}
