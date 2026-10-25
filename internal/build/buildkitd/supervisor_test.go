package buildkitd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

const testImage = "docker.io/moby/buildkit:v0.34.1-rootless@sha256:" +
	"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type fakeRT struct {
	st      *docker.ContainerState
	calls   []string
	created docker.ContainerSpec
}

func (f *fakeRT) InspectByName(context.Context, string) (*docker.ContainerState, error) {
	return f.st, nil
}

func (f *fakeRT) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	f.calls = append(f.calls, "create")
	f.created = spec
	f.st = &docker.ContainerState{ID: "c1", Image: spec.Image}
	return "c1", nil
}

func (f *fakeRT) Start(context.Context, string) error {
	f.calls = append(f.calls, "start")
	f.st.Running = true
	return nil
}

func (f *fakeRT) Stop(context.Context, string, time.Duration) error {
	f.calls = append(f.calls, "stop")
	f.st.Running = false
	return nil
}

func (f *fakeRT) Remove(context.Context, string, bool) error {
	f.calls = append(f.calls, "remove")
	f.st = nil
	return nil
}

func (f *fakeRT) EnsureVolume(context.Context, string) error { return nil }

func newSup(t *testing.T, rt *fakeRT) (*Supervisor, *time.Time) {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{
		Image: testImage, Snapshotter: "auto", GCKeepMB: 100, GCKeepFor: time.Hour,
		HealthTimeout: time.Second, IdleStop: time.Minute,
		SocketDir: filepath.Join(dir, "run"), ConfigDir: filepath.Join(dir, "conf"),
	}
	s, err := New(cfg, rt)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Unix(1000, 0)
	s.now = func() time.Time { return clock }
	s.socketOK = func(string) bool { return true }
	s.health = func(context.Context, string, time.Duration) error { return nil }
	return s, &clock
}

func TestBuildkitdCreatesWithHardenedSpec(t *testing.T) {
	rt := &fakeRT{}
	s, _ := newSup(t, rt)
	addr, err := s.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(addr, "unix://") || strings.Join(rt.calls, ",") != "create,start" {
		t.Fatalf("addr %q calls %v", addr, rt.calls)
	}
	sp := rt.created
	if sp.NetworkMode != "none" || len(sp.Ports) != 0 || sp.Labels["levelrail.role"] != "buildkitd" {
		t.Fatalf("spec not locked down: %+v", sp)
	}
	fi, err := os.Stat(s.cfg.SocketDir)
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("socket dir mode %v err %v", fi, err)
	}
	body, _ := os.ReadFile(filepath.Join(s.cfg.ConfigDir, "buildkitd.toml"))
	if !strings.Contains(string(body), "keepBytes = 104857600") {
		t.Fatalf("toml: %s", body)
	}
}

func TestBuildkitdReplacesOnImageMismatch(t *testing.T) {
	rt := &fakeRT{st: &docker.ContainerState{ID: "old", Image: "other@sha256:x", Running: true}}
	s, _ := newSup(t, rt)
	if _, err := s.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(rt.calls, ","); got != "stop,remove,create,start" {
		t.Fatalf("calls %s", got)
	}
}

func TestBuildkitdStartsStoppedContainer(t *testing.T) {
	rt := &fakeRT{st: &docker.ContainerState{ID: "c1", Image: testImage}}
	s, _ := newSup(t, rt)
	if _, err := s.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(rt.calls, ","); got != "start" {
		t.Fatalf("calls %s", got)
	}
}

func TestBuildkitdRestartsOnceWhenSocketMissing(t *testing.T) {
	rt := &fakeRT{st: &docker.ContainerState{ID: "c1", Image: testImage, Running: true}}
	s, _ := newSup(t, rt)
	checks := 0
	s.socketOK = func(string) bool { checks++; return checks > 1 }
	if _, err := s.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(rt.calls, ","); got != "stop,start" {
		t.Fatalf("calls %s", got)
	}
}

func TestBuildkitdUnhealthyAfterRestartFails(t *testing.T) {
	rt := &fakeRT{st: &docker.ContainerState{ID: "c1", Image: testImage, Running: true}}
	s, _ := newSup(t, rt)
	s.health = func(context.Context, string, time.Duration) error { return errors.New("down") }
	_, err := s.Acquire(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err %v", err)
	}
	if got := strings.Join(rt.calls, ","); got != "stop,start" {
		t.Fatalf("calls %s", got)
	}
}

func TestBuildkitdSocketGoneContainerMissingRecreates(t *testing.T) {
	rt := &fakeRT{}
	s, _ := newSup(t, rt)
	if _, err := s.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.created.Name != ContainerName {
		t.Fatal("not created")
	}
}

func TestBuildkitdIdleStop(t *testing.T) {
	rt := &fakeRT{}
	s, clock := newSup(t, rt)
	if _, err := s.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	*clock = clock.Add(2 * time.Minute)
	if stopped, _ := s.IdleStop(context.Background()); stopped {
		t.Fatal("stopped while a build is in flight")
	}
	s.Release()
	if stopped, _ := s.IdleStop(context.Background()); stopped {
		t.Fatal("stopped before idle window")
	}
	*clock = clock.Add(2 * time.Minute)
	if stopped, err := s.IdleStop(context.Background()); err != nil || !stopped {
		t.Fatalf("stopped %v err %v", stopped, err)
	}
	s.cfg.IdleStop = 0
	rt.st.Running = true
	if stopped, _ := s.IdleStop(context.Background()); stopped {
		t.Fatal("idle stop 0 must keep running")
	}
}

func TestBuildkitdRefusesUnpinnedImage(t *testing.T) {
	if _, err := New(Config{Image: "moby/buildkit:v0.34.1-rootless"}, &fakeRT{}); !errors.Is(err, ErrImageNotPinned) {
		t.Fatalf("err %v", err)
	}
}

func TestBuildkitdConfigFromEnv(t *testing.T) {
	env := map[string]string{
		"XDG_RUNTIME_DIR": "/run/user/1000", "APP_BUILDKIT_IDLE_STOP": "0",
		"APP_BUILDKIT_GC_KEEP_MB": "512", "APP_BUILDKIT_SNAPSHOTTER": "fuse-overlayfs",
	}
	cfg, err := ConfigFromEnv(func(k string) string { return env[k] }, true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IdleStop != 0 || cfg.GCKeepMB != 512 || cfg.SocketDir != "/run/user/1000/levelrail/buildkit" {
		t.Fatalf("%+v", cfg)
	}
	if _, err := ConfigFromEnv(func(string) string { return "" }, false); err == nil {
		t.Fatal("rootful without APP_DATA_DIR must fail")
	}
}
