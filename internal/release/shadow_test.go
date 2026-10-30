package release

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seed(t *testing.T, db *store.DB, id, digest string) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: id, ServiceName: "web", Image: "reg.example/web:v1", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusRunning, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if digest != "" {
		if err := db.SetDeployAttemptDigest(ctx, id, digest, store.DigestReasonStaticTree); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.FinishDeployAttempt(ctx, id, store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		t.Fatal(err)
	}
}

func open(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestShadowWritesOneReleasePerDigestAttempt(t *testing.T) {
	db := open(t)
	Install(db, ModeShadow, nil)
	seed(t, db, "dep_a", "sha256:aaa")
	seed(t, db, "dep_nodigest", "")
	// Replaying the hook must not duplicate the release.
	NewShadow(db, ModeShadow, nil).OnAttempt(context.Background(), "dep_a")

	rels, err := db.ListReleasesByApp(context.Background(), "web")
	if err != nil || len(rels) != 1 {
		t.Fatalf("releases = %d, err = %v, want 1", len(rels), err)
	}
	if rels[0].ImageRef != "reg.example/web@sha256:aaa" {
		t.Fatalf("image ref = %q", rels[0].ImageRef)
	}
	b, err := db.GetEnvironmentBinding(context.Background(), "web")
	if err != nil || b == nil || b.ReleaseID != rels[0].ID {
		t.Fatalf("binding = %+v, err = %v", b, err)
	}
	a, err := db.GetDeployAttempt(context.Background(), "dep_a")
	if err != nil || a.Status != store.DeployAttemptStatusSucceeded {
		t.Fatalf("deploy outcome changed: %+v %v", a, err)
	}
}

func TestOffModeWritesNothing(t *testing.T) {
	db := open(t)
	Install(db, ModeOff, nil)
	seed(t, db, "dep_a", "sha256:aaa")
	NewShadow(db, ModeOff, nil).OnAttempt(context.Background(), "dep_a")

	rels, err := db.ListReleasesByApp(context.Background(), "web")
	if err != nil || len(rels) != 0 {
		t.Fatalf("releases = %d, err = %v, want 0", len(rels), err)
	}
	if b, _ := db.GetEnvironmentBinding(context.Background(), "web"); b != nil {
		t.Fatalf("binding written in off mode: %+v", b)
	}
}
