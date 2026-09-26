package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
)

type hookBuilder struct {
	fakeBuilder
	afterBuild func()
}

func (h *hookBuilder) Build(ctx context.Context, req build.Request, progress func(build.ProgressEvent)) (*build.Result, error) {
	res, err := h.fakeBuilder.Build(ctx, req, progress)
	if h.afterBuild != nil {
		h.afterBuild()
	}
	if err == nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return res, err
}

func cancelRequest(reg *CancelRegistry, id string) Request {
	return Request{
		ServiceName: "web", Service: dockerfileService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
		AttemptID: id, Commit: func() error { return reg.Commit(id) },
	}
}

func TestCancelRegistry_Transitions(t *testing.T) {
	reg := NewCancelRegistry()
	if got := reg.Cancel("dep_x", "u"); got != CancelUnknown {
		t.Fatalf("unknown id = %v", got)
	}
	reg.Register("dep_1")
	if got := reg.Cancel("dep_1", "alice"); got != CancelRequested {
		t.Fatalf("cancel = %v", got)
	}
	if by, ok := reg.Canceled("dep_1"); !ok || by != "alice" {
		t.Fatalf("canceled = %q, %v", by, ok)
	}
	if err := reg.Commit("dep_1"); !errors.Is(err, ErrCanceled) {
		t.Fatalf("commit after cancel = %v", err)
	}

	reg.Register("dep_2")
	if err := reg.Commit("dep_2"); err != nil {
		t.Fatal(err)
	}
	if got := reg.Cancel("dep_2", "bob"); got != CancelTooLate {
		t.Fatalf("cancel after commit = %v", got)
	}
	if _, ok := reg.Canceled("dep_2"); ok {
		t.Fatal("a too-late cancel must not mark the deploy canceled")
	}
	reg.Release("dep_2")
	if got := reg.Cancel("dep_2", "bob"); got != CancelUnknown {
		t.Fatalf("cancel after release = %v", got)
	}
}

func TestCancelRegistry_BindAfterCancelIsAlreadyDone(t *testing.T) {
	reg := NewCancelRegistry()
	reg.Register("dep_1")
	reg.Cancel("dep_1", "alice")
	if reg.Bind(context.Background(), "dep_1").Err() == nil {
		t.Fatal("a context bound after a cancel must already be canceled")
	}
	if reg.Bind(context.Background(), "dep_missing").Err() != nil {
		t.Fatal("an unregistered id must get its parent back")
	}
}

func TestPipeline_Deploy_CancelDuringBuild_DoesNotSave(t *testing.T) {
	reg := NewCancelRegistry()
	reg.Register("dep_1")
	builder := &hookBuilder{fakeBuilder: fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}}
	builder.afterBuild = func() { reg.Cancel("dep_1", "alice") }
	svcStore := &fakeServiceStore{}
	p := New(builder, svcStore)

	ctx := reg.Bind(context.Background(), "dep_1")
	_, err := p.Deploy(ctx, cancelRequest(reg, "dep_1"), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if svcStore.saveCalls != 0 {
		t.Fatalf("SaveDesiredService called %d times, want 0", svcStore.saveCalls)
	}
}

func TestPipeline_Deploy_CancelBetweenBuildAndCommit_DoesNotSave(t *testing.T) {
	reg := NewCancelRegistry()
	reg.Register("dep_1")
	builder := &hookBuilder{fakeBuilder: fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}}
	svcStore := &fakeServiceStore{}
	p := New(builder, svcStore)
	req := cancelRequest(reg, "dep_1")
	req.Commit = func() error {
		reg.Cancel("dep_1", "alice")
		return reg.Commit("dep_1")
	}

	_, err := p.Deploy(context.Background(), req, nil)
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("err = %v, want ErrCanceled", err)
	}
	if svcStore.saveCalls != 0 {
		t.Fatalf("a build that finished but was canceled must not write desired state, saved %d times", svcStore.saveCalls)
	}
}

func TestPipeline_Deploy_CancelAfterCommit_TooLateAndSaved(t *testing.T) {
	reg := NewCancelRegistry()
	reg.Register("dep_1")
	builder := &hookBuilder{fakeBuilder: fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}}
	svcStore := &fakeServiceStore{}
	p := New(builder, svcStore)

	if _, err := p.Deploy(context.Background(), cancelRequest(reg, "dep_1"), nil); err != nil {
		t.Fatal(err)
	}
	if got := reg.Cancel("dep_1", "alice"); got != CancelTooLate {
		t.Fatalf("cancel = %v, want CancelTooLate", got)
	}
	if svcStore.saveCalls != 1 {
		t.Fatalf("saveCalls = %d, want 1", svcStore.saveCalls)
	}
}
