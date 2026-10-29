package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/release"
	"github.com/GLINCKER/levelrail/internal/store"
)

const pipelineDigest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"

type nopNudger struct{}

func (nopNudger) Nudge() {}

func seedPipelinePromotion(t *testing.T) (*pipelineActions, *store.DB, *store.Release) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	release.Install(db, release.ModeOn, nil)
	for _, svc := range []store.DesiredService{
		{Name: "web-staging", Image: "levelrail/web:2", Port: 3000},
		{Name: "web-prod", Image: "levelrail/web:1", Port: 3000},
	} {
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: "src1", ServiceName: "web-staging", Image: "levelrail/web:2", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusRunning, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDeployAttemptDigest(ctx, "src1", pipelineDigest, store.DigestReasonResolved); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployAttempt(ctx, "src1", store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		t.Fatal(err)
	}
	rel, err := db.GetReleaseByAttempt(ctx, "src1")
	if err != nil || rel == nil {
		t.Fatalf("release = %v, %v", rel, err)
	}
	a := &pipelineActions{db: db, nudger: nopNudger{}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return a, db, rel
}

func TestPipelinePromoteBindingsOnRetargetsAndDeploysPinnedRef(t *testing.T) {
	t.Setenv(release.EnvBindings, "on")
	a, db, rel := seedPipelinePromotion(t)
	ctx := context.Background()

	image, err := a.Promote(ctx, "web-staging", "web-prod", func(string) {})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if image != rel.ImageRef || !strings.Contains(image, "@"+pipelineDigest) {
		t.Errorf("image = %q, want pinned ref %q", image, rel.ImageRef)
	}
	b, _ := db.GetEnvironmentBinding(ctx, "web-prod")
	if b == nil || b.ReleaseID != rel.ID {
		t.Errorf("binding = %+v, want release %s", b, rel.ID)
	}
	svc, _ := db.GetDesiredService(ctx, "web-prod")
	if svc.Image != rel.ImageRef {
		t.Errorf("desired image = %q, want %q", svc.Image, rel.ImageRef)
	}
}

func TestPipelinePromoteBindingsOffKeepsTag(t *testing.T) {
	t.Setenv(release.EnvBindings, "off")
	a, db, _ := seedPipelinePromotion(t)
	ctx := context.Background()

	image, err := a.Promote(ctx, "web-staging", "web-prod", func(string) {})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if image != "levelrail/web:2" {
		t.Errorf("image = %q, want the tag", image)
	}
	if b, _ := db.GetEnvironmentBinding(ctx, "web-prod"); b != nil {
		t.Errorf("binding written while off: %+v", b)
	}
}
