package deploywatch

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeImages struct {
	tags     map[string]string
	untagged []string
	tagErr   error
}

func (f *fakeImages) TagImage(_ context.Context, source, target string) error {
	if f.tagErr != nil {
		return f.tagErr
	}
	f.tags[target] = source
	return nil
}

func (f *fakeImages) UntagImage(_ context.Context, ref string) error {
	delete(f.tags, ref)
	f.untagged = append(f.untagged, ref)
	return nil
}

func TestPinRef(t *testing.T) {
	if got := PinRef("My Brand", "web.api", "dep_Ab-9"); got != "my-brand-rollback/web.api:dep_Ab-9" {
		t.Errorf("PinRef() = %q", got)
	}
	if got := PinRef("", "web", "dep_1"); got != "app-rollback/web:dep_1" {
		t.Errorf("PinRef() with no namespace = %q", got)
	}
}

func TestPinner_PinsKeepsNewestAndSkipsRemote(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	images := &fakeImages{tags: map[string]string{}}
	p := &Pinner{Store: db, Images: images, Keep: 2, Namespace: "x", LocalNodeID: "local"}

	base := time.Now().Add(-time.Hour)
	for i, tag := range []string{"v1", "v2", "v3"} {
		svc := store.DesiredService{Name: "web", Image: "app:" + tag}
		saveAttempt(t, db, "dep_"+tag, svc, base.Add(time.Duration(i)*time.Minute))
		ref, err := p.PinServing(ctx, svc, "sha256:"+tag)
		if err != nil || ref != PinRef("x", "web", "dep_"+tag) {
			t.Fatalf("PinServing(%s) = %q, %v", tag, ref, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	if again, _ := p.PinServing(ctx, store.DesiredService{Name: "web", Image: "app:v3"}, "sha256:v3"); again != "" {
		t.Errorf("re-pinning an already pinned attempt returned %q, want no-op", again)
	}
	pins, _ := db.ListRollbackPins(ctx, "web")
	if len(pins) != 2 || pins[0].DeployAttemptID != "dep_v3" || pins[1].DeployAttemptID != "dep_v2" {
		t.Errorf("pins = %+v, want the newest two", pins)
	}
	if len(images.untagged) != 1 || images.untagged[0] != PinRef("x", "web", "dep_v1") {
		t.Errorf("untagged = %v, want only v1's pin", images.untagged)
	}
	if images.tags[PinRef("x", "web", "dep_v3")] != "sha256:v3" {
		t.Errorf("v3 pin points at %q, want its image ID", images.tags[PinRef("x", "web", "dep_v3")])
	}

	if ref, err := p.PinServing(ctx, store.DesiredService{Name: "web", Image: "app:v3", NodeID: "remote"}, "sha256:v3"); ref != "" || err != nil {
		t.Errorf("remote node PinServing() = %q, %v, want skipped", ref, err)
	}

	images.tagErr = errors.New("boom")
	saveAttempt(t, db, "dep_v4", store.DesiredService{Name: "web", Image: "app:v4"}, time.Now())
	if _, err := p.PinServing(ctx, store.DesiredService{Name: "web", Image: "app:v4"}, "sha256:v4"); err == nil {
		t.Error("PinServing() with a failing tag returned nil error")
	}
	if pin, _ := db.GetRollbackPin(ctx, "web", "dep_v4"); pin != nil {
		t.Errorf("a pin row was saved although tagging failed: %+v", pin)
	}
}
