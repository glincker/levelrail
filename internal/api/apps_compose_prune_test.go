package api

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPruneStaleComposeServicesClearsSecrets(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	mgr := secrets.NewManager(db, mk)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithSecretSetter(mgr))
	seedApp(t, db, "web")
	seedApp(t, db, "worker")
	ctx := context.Background()
	if err := mgr.SetValue(ctx, "worker", "TOKEN", "s3cret"); err != nil {
		t.Fatal(err)
	}
	prev := []store.DesiredService{{Name: "web"}, {Name: "worker"}}
	rt.pruneStaleComposeServices(ctx, "web", prev, prev[:1])
	if _, err := mgr.Resolve(ctx, "worker", "TOKEN"); !errors.Is(err, secrets.ErrValueNotFound) {
		t.Fatalf("pruned service secret still resolvable: %v", err)
	}
}
