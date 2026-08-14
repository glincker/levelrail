package build

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
)

// collectProgress is a hand-written fake progress sink, recording every
// event so a test can assert real events actually flowed through, not
// just that Build() didn't error.
type collectProgress struct {
	mu     sync.Mutex
	events []ProgressEvent
}

func (c *collectProgress) fn() func(ProgressEvent) {
	return func(ev ProgressEvent) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.events = append(c.events, ev)
	}
}

func (c *collectProgress) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

func TestSlogProgress(_ *testing.T) {
	// A lightweight smoke test: SlogProgress is a formatting adapter, not
	// business logic, so this just proves it doesn't panic on any of the
	// event shapes Build actually produces, including a nil logger.
	fn := SlogProgress(nil)
	fn(ProgressEvent{Step: "build", Error: "boom", Completed: true})
	fn(ProgressEvent{Step: "build", Cached: true, Completed: true})
	fn(ProgressEvent{Log: "some output"})
	fn(ProgressEvent{}) // no-op shape: none of Error/Completed/Log set
}

// liveClient dials the local Docker Engine and, through it, the embedded
// BuildKit instance (see client.go for why that indirection exists). It
// skips the calling test cleanly, rather than failing, when either isn't
// reachable, so this test suite does not require Docker-in-Docker to pass
// in CI, per this codebase's testing standard.
func liveClient(t *testing.T, opts ...Option) (*dockerclient.Client, *Client) {
	t.Helper()

	docker, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		t.Skipf("skipping: could not construct docker client: %v", err)
	}
	t.Cleanup(func() { _ = docker.Close() })

	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := docker.Ping(pingCtx); err != nil {
		t.Skipf("skipping: no local docker daemon reachable: %v", err)
	}

	connectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	bk, err := NewClient(connectCtx, docker, opts...)
	if err != nil {
		t.Skipf("skipping: could not connect to buildkit via docker: %v", err)
	}
	t.Cleanup(func() { _ = bk.Close() })

	if _, err := bk.bk.Info(connectCtx); err != nil {
		t.Skipf("skipping: buildkit connection did not respond to Info: %v", err)
	}

	return docker, bk
}

// TestClient_Build_Live is the Phase 0 spike's actual proof: build the
// testdata Dockerfile through BuildKit's Go client, with no `docker
// build` or `docker` CLI involved, and confirm the resulting image really
// exists in the local Docker image store by asking the Engine API
// directly (not by re-listing through our own Build code path).
func TestClient_Build_Live(t *testing.T) {
	docker, bk := liveClient(t)

	tag := "levelrail-buildkit-spike:test"
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = docker.ImageRemove(cleanupCtx, tag, image.RemoveOptions{Force: true})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	progress := &collectProgress{}
	res, err := bk.Build(ctx, Request{
		ContextDir: "testdata",
		Tag:        tag,
	}, progress.fn())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if res.Tag != tag {
		t.Errorf("Result.Tag = %q, want %q", res.Tag, tag)
	}
	if res.Duration <= 0 {
		t.Errorf("Result.Duration = %v, want > 0", res.Duration)
	}
	if progress.count() == 0 {
		t.Error("expected at least one ProgressEvent from a real build, got none")
	}

	// Verify independently, through the Docker Engine API, that the image
	// actually landed in the local image store. This is the load-bearing
	// assertion for the whole spike: it proves the tar-stream export plus
	// /images/load round trip really works, not just that BuildKit's
	// Solve call returned without error.
	inspect, _, err := docker.ImageInspectWithRaw(ctx, tag)
	if err != nil {
		t.Fatalf("image %q not found after Build(): %v", tag, err)
	}
	if inspect.ID == "" {
		t.Error("inspected image has an empty ID")
	}
}

// TestClient_Build_Live_InvalidRequest exercises the request-validation
// error path through the live client without needing the build to
// actually run, distinguishing "buildkit unreachable" (skip) from
// "request rejected before any network call" (a real assertion).
func TestClient_Build_Live_InvalidRequest(t *testing.T) {
	_, bk := liveClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := bk.Build(ctx, Request{}, nil)
	if !errors.Is(err, ErrContextDirRequired) {
		t.Fatalf("Build() with empty request error = %v, want %v", err, ErrContextDirRequired)
	}
}

// TestClient_Build_Live_CacheAccelerates proves WithCacheDir isn't just
// wired but actually does something: a second build of the same
// Dockerfile and context, with the cache from the first build available,
// reports at least one cached step. A faster wall-clock time alone
// wouldn't rule out noise; a real cached=true event is the load-bearing
// assertion.
func TestClient_Build_Live_CacheAccelerates(t *testing.T) {
	cacheDir := t.TempDir()
	docker, bk := liveClient(t, WithCacheDir(cacheDir))

	tag := "levelrail-buildkit-spike:cache-test"
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = docker.ImageRemove(cleanupCtx, tag, image.RemoveOptions{Force: true})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	req := Request{ContextDir: "testdata", Tag: tag}

	if _, err := bk.Build(ctx, req, nil); err != nil {
		t.Fatalf("first (cold) Build() error = %v", err)
	}

	second := &collectProgress{}
	if _, err := bk.Build(ctx, req, second.fn()); err != nil {
		t.Fatalf("second (warm) Build() error = %v", err)
	}

	sawCached := false
	for _, ev := range second.events {
		if ev.Completed && ev.Cached {
			sawCached = true
			break
		}
	}
	if !sawCached {
		t.Error("expected at least one cached=true step on the second build, got none; WithCacheDir may not be taking effect")
	}
}

// TestClient_Build_Live_CacheRegistryUnreachable exercises TASKS.md
// 3.5's other documented failure mode: a registry cache backend
// (WithCacheRegistry) pointed at a host that will never resolve. This
// must surface as a real Build() error, not a silent "cache just didn't
// help this time": an operator who configured a dedicated build node's
// registry cache wrong needs to see that clearly rather than discover
// it only as an unexplained slow build.
func TestClient_Build_Live_CacheRegistryUnreachable(t *testing.T) {
	_, bk := liveClient(t, WithCacheRegistry("build-cache.invalid.example/levelrail/cache:app"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := bk.Build(ctx, Request{ContextDir: "testdata", Tag: "levelrail-buildkit-spike:cache-unreachable"}, nil)
	if err == nil {
		t.Fatal("Build() with an unreachable cache registry: error = nil, want a non-nil error")
	}
}
