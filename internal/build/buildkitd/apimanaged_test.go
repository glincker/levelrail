package buildkitd

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

const testPinned = "docker.io/moby/buildkit:v0.34.1-rootless@sha256:0000000000000000000000000000000000000000000000000000000000000000"

type fakeEngine struct {
	obs        *Observed
	obsAfter   *Observed // returned after the first create attempt
	createErr  error
	pullErr    error
	hasImage   bool
	vol        map[string]string
	volFound   bool
	created    []CreateSpec
	started    []string
	pulled     int
	volCreated int
}

func (f *fakeEngine) InspectContainer(context.Context, string) (*Observed, error) {
	if len(f.created) > 0 && f.obsAfter != nil {
		return f.obsAfter, nil
	}
	return f.obs, nil
}
func (f *fakeEngine) CreateContainer(_ context.Context, s CreateSpec) (string, error) {
	f.created = append(f.created, s)
	return "new", f.createErr
}
func (f *fakeEngine) StartContainer(_ context.Context, id string) error {
	f.started = append(f.started, id)
	return nil
}
func (f *fakeEngine) PutFiles(context.Context, string, string, io.Reader) error { return nil }
func (f *fakeEngine) HasImage(context.Context, string) (bool, error)            { return f.hasImage, nil }
func (f *fakeEngine) PullImage(context.Context, string) error {
	f.pulled++
	return f.pullErr
}
func (f *fakeEngine) InspectVolume(context.Context, string) (map[string]string, bool, error) {
	return f.vol, f.volFound, nil
}
func (f *fakeEngine) CreateVolume(context.Context, string, map[string]string) error {
	f.volCreated++
	return nil
}

func goodObserved(running bool) *Observed {
	return &Observed{ID: "old", Image: testPinned, Running: running, Labels: managedLabels(),
		Mounts: []Mount{{Name: VolumeName, Destination: rootlessDataPath}}}
}

func newMgr(t *testing.T, f *fakeEngine) *APIManager {
	t.Helper()
	f.vol, f.volFound = managedLabels(), true
	m, err := NewAPIManager(Config{Image: testPinned}, f)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAPIManagerRefusesUnpinned(t *testing.T) {
	if _, err := NewAPIManager(Config{Image: "moby/buildkit:v1-rootless"}, &fakeEngine{}); !errors.Is(err, ErrImageNotPinned) {
		t.Fatalf("got %v", err)
	}
}

func TestAPIManagerRefusesUnlabeledReuse(t *testing.T) {
	f := &fakeEngine{obs: &Observed{ID: "x", Image: testPinned, Running: true}}
	if err := newMgr(t, f).Ensure(context.Background()); !errors.Is(err, ErrForeign) {
		t.Fatalf("got %v", err)
	}
	if len(f.created) != 0 || len(f.started) != 0 {
		t.Fatal("must not touch a foreign container")
	}
}

func TestAPIManagerRefusesWrongImageAndMounts(t *testing.T) {
	wrongImg := goodObserved(true)
	wrongImg.Image = "other"
	noMount := goodObserved(true)
	noMount.Mounts = nil
	priv := goodObserved(true)
	priv.Privileged = true
	for name, o := range map[string]*Observed{"image": wrongImg, "mounts": noMount, "privileged": priv} {
		if err := newMgr(t, &fakeEngine{obs: o}).Ensure(context.Background()); !errors.Is(err, ErrForeign) {
			t.Fatalf("%s: got %v", name, err)
		}
	}
}

func TestAPIManagerRefusesUnlabeledVolume(t *testing.T) {
	f := &fakeEngine{}
	m := newMgr(t, f)
	f.vol = map[string]string{}
	if err := m.Ensure(context.Background()); !errors.Is(err, ErrForeign) {
		t.Fatalf("got %v", err)
	}
}

func TestAPIManagerCreatesVolumeWithLabelAndNeverPrivileged(t *testing.T) {
	f := &fakeEngine{hasImage: true}
	m := newMgr(t, f)
	f.volFound = false
	if err := m.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.volCreated != 1 || len(f.created) != 1 || len(f.started) != 1 {
		t.Fatalf("vol=%d created=%d started=%d", f.volCreated, len(f.created), len(f.started))
	}
	if !strings.Contains(strings.Join(f.created[0].SecurityOpt, ","), "seccomp=unconfined") {
		t.Fatalf("rootless security opts missing: %v", f.created[0].SecurityOpt)
	}
}

func TestAPIManagerCreateConflictReinspectsAndStarts(t *testing.T) {
	f := &fakeEngine{hasImage: true, createErr: ErrConflict, obsAfter: goodObserved(false)}
	if err := newMgr(t, f).Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.started) != 1 || f.started[0] != "old" {
		t.Fatalf("started %v", f.started)
	}
}

func TestAPIManagerConflictWithForeignWinnerRefused(t *testing.T) {
	f := &fakeEngine{hasImage: true, createErr: ErrConflict, obsAfter: &Observed{ID: "z", Image: testPinned}}
	if err := newMgr(t, f).Ensure(context.Background()); !errors.Is(err, ErrForeign) {
		t.Fatalf("got %v", err)
	}
}

func TestAPIManagerPullErrorSurfaces(t *testing.T) {
	f := &fakeEngine{pullErr: errors.New("denied")}
	err := newMgr(t, f).Ensure(context.Background())
	if err == nil || !strings.Contains(err.Error(), "denied") || len(f.created) != 0 {
		t.Fatalf("err=%v created=%d", err, len(f.created))
	}
}

func TestDecodePullStreamSurfacesError(t *testing.T) {
	stream := `{"status":"Pulling"}` + "\n" + `{"errorDetail":{"message":"manifest unknown"},"error":"manifest unknown"}`
	if err := DecodePullStream(strings.NewReader(stream)); err == nil || err.Error() != "manifest unknown" {
		t.Fatalf("got %v", err)
	}
	if err := DecodePullStream(strings.NewReader(`{"status":"ok"}`)); err != nil {
		t.Fatal(err)
	}
}
