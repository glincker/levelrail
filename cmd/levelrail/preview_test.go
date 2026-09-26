package main

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type recordingRollouts struct {
	calls int
	err   error
}

func (r *recordingRollouts) RecordRollout(context.Context, string, string, string, string) error {
	r.calls++
	return r.err
}

type recordingNotifier struct{ got [][3]string }

func (n *recordingNotifier) NotifyReady(app, image, runningImageID string) {
	n.got = append(n.got, [3]string{app, image, runningImageID})
}

func TestPreviewRolloutRecorder_NotifiesOnlyWhenServing(t *testing.T) {
	inner := &recordingRollouts{}
	n := &recordingNotifier{}
	rec := previewRolloutRecorder{inner: inner, notify: n}

	if err := rec.RecordRollout(context.Background(), "web", "img:1", store.RolloutStateMismatch, "sha256:x"); err != nil {
		t.Fatal(err)
	}
	if len(n.got) != 0 {
		t.Errorf("mismatch must not trigger a preview, got %v", n.got)
	}
	if err := rec.RecordRollout(context.Background(), "web", "img:1", store.RolloutStateServing, "sha256:x"); err != nil {
		t.Fatal(err)
	}
	if len(n.got) != 1 || n.got[0] != [3]string{"web", "img:1", "sha256:x"} {
		t.Errorf("notified = %v, want web img:1 sha256:x", n.got)
	}
	if inner.calls != 2 {
		t.Errorf("inner calls = %d, want every call forwarded", inner.calls)
	}
}

func TestPreviewRolloutRecorder_InnerErrorStillPropagates(t *testing.T) {
	boom := errors.New("boom")
	rec := previewRolloutRecorder{inner: &recordingRollouts{err: boom}, notify: &recordingNotifier{}}
	if err := rec.RecordRollout(context.Background(), "web", "img:1", store.RolloutStateServing, ""); !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
}
