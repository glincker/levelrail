package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/failure"
)

// Disk full during a build: the failure is typed as disk_full, the stored
// desired state (what the running release was built from) is not replaced,
// and a retry once space is freed saves exactly once.
func TestChaosPack_DiskFullDuringBuild_TypedNoPartialStateAndRetryConverges(t *testing.T) {
	builder := &fakeBuilder{err: errors.New("failed to solve: write /var/lib/buildkit/x: no space left on device")}
	svcStore := &fakeServiceStore{}
	p := New(builder, svcStore)
	req := Request{ServiceName: "web", Service: dockerfileService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}

	_, err := p.Deploy(context.Background(), req, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want the disk-full failure")
	}
	got, ok := failure.Classify(failure.Input{Status: "failed", Error: err.Error()}, failure.Options{})
	if !ok || got.Code != failure.CodeDiskFull {
		t.Fatalf("Classify(%q) = %q, %v, want %q", err, got.Code, ok, failure.CodeDiskFull)
	}
	if svcStore.saveCalls != 0 {
		t.Fatalf("SaveDesiredService calls = %d, want 0: a failed build must leave the serving release's desired state alone", svcStore.saveCalls)
	}

	builder.err = nil
	builder.result = &build.Result{Tag: "levelrail/web:abc1234"}
	if _, err := p.Deploy(context.Background(), req, nil); err != nil {
		t.Fatalf("Deploy() after space was freed error = %v", err)
	}
	if svcStore.saveCalls != 1 {
		t.Fatalf("SaveDesiredService calls = %d, want exactly 1", svcStore.saveCalls)
	}
	if _, err := p.Deploy(context.Background(), req, nil); err != nil {
		t.Fatalf("repeat Deploy() error = %v", err)
	}
	if svcStore.saved.Image != "levelrail/web:abc1234" {
		t.Fatalf("saved image = %q, want the same tag on a repeat deploy", svcStore.saved.Image)
	}
}

// A corrupt image layer surfaced by the image load step is typed and saves nothing.
func TestChaosPack_CorruptImageAtLoad_TypedAndNothingSaved(t *testing.T) {
	builder := &fakeBuilder{err: errors.New("load image: failed to register layer: archive/tar: invalid tar header")}
	svcStore := &fakeServiceStore{}
	_, err := New(builder, svcStore).Deploy(context.Background(), Request{
		ServiceName: "web", Service: dockerfileService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
	}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want the corrupt layer failure")
	}
	got, ok := failure.Classify(failure.Input{Status: "failed", Error: err.Error()}, failure.Options{})
	if !ok || got.Code != failure.CodeImageCorrupt {
		t.Fatalf("Classify(%q) = %q, %v, want %q", err, got.Code, ok, failure.CodeImageCorrupt)
	}
	if svcStore.saveCalls != 0 {
		t.Fatalf("SaveDesiredService calls = %d, want 0", svcStore.saveCalls)
	}
}
