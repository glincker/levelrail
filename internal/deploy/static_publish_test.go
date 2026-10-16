package deploy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeStaticRecorder struct {
	digest, reason, rollout string
	events                  []store.DeployGateEvent
}

func (f *fakeStaticRecorder) SetDeployAttemptDigest(_ context.Context, _, d, r string) error {
	f.digest, f.reason = d, r
	return nil
}

func (f *fakeStaticRecorder) SetDeployAttemptRollout(_ context.Context, _, s string) error {
	f.rollout = s
	return nil
}

func (f *fakeStaticRecorder) RecordGateEvent(_ context.Context, e store.DeployGateEvent) error {
	f.events = append(f.events, e)
	return nil
}

func TestPipeline_DeployStatic_RecordsPublishAndBuildLog(t *testing.T) {
	rec := &fakeStaticRecorder{}
	p := New(&fakeBuilder{}, &fakeServiceStore{}, WithStaticSiteStore(&fakeStaticSiteStore{}),
		WithStaticRootDir(t.TempDir()), WithStaticPublishRecorder(rec))
	src := t.TempDir()
	writeTree(t, src, map[string]string{"dist/index.html": "<h1>hi</h1>"})

	var events []build.ProgressEvent
	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "docs", Service: staticService("docs.example.com"), SourceDir: src, CommitSHA: "abc1234", AttemptID: "dep_1",
	}, func(e build.ProgressEvent) { events = append(events, e) })
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if !strings.HasPrefix(rec.digest, "sha256:") || rec.reason != store.DigestReasonStaticTree {
		t.Errorf("digest = %q reason = %q, want sha256 static tree", rec.digest, rec.reason)
	}
	if rec.rollout != store.RolloutStateServing {
		t.Errorf("rollout = %q, want serving", rec.rollout)
	}
	var phases []string
	for _, e := range rec.events {
		phases = append(phases, e.Phase+":"+e.Status)
	}
	got := strings.Join(phases, ",")
	if want := deploysafety.PhaseCutover + ":passed," + deploysafety.PhaseWatch + ":passed"; got != want {
		t.Errorf("gate events = %s, want %s", got, want)
	}
	if stage := deploysafety.Stage(rec.events); stage != deploysafety.StageCleared {
		t.Errorf("stage = %q, want cleared (terminal)", stage)
	}
	steps := map[string]bool{}
	for _, e := range events {
		if e.Completed {
			steps[e.Step] = true
		}
	}
	if !steps[stepStaticFetch] || !steps[stepStaticPublish] {
		t.Errorf("completed steps = %v, want fetch and publish", steps)
	}
}

func TestPipeline_DeployStatic_SaveFailure_RecordsNothingServed(t *testing.T) {
	rec := &fakeStaticRecorder{}
	p := New(&fakeBuilder{}, &fakeServiceStore{}, WithStaticSiteStore(&fakeStaticSiteStore{saveErr: errors.New("disk full")}),
		WithStaticRootDir(t.TempDir()), WithStaticPublishRecorder(rec))
	src := t.TempDir()
	writeTree(t, src, map[string]string{"dist/index.html": "x"})

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "docs", Service: staticService("docs.example.com"), SourceDir: src, CommitSHA: "abc", AttemptID: "dep_1",
	}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want save error")
	}
	if rec.rollout != "" || rec.digest != "" || len(rec.events) != 0 {
		t.Errorf("recorded %+v after a failed publish, want nothing served", rec)
	}
}

func TestHashStaticTree(t *testing.T) {
	tests := []struct {
		name      string
		a, b      map[string]string
		wantEqual bool
	}{
		{"same content", map[string]string{"i.html": "a", "d/x.css": "b"}, map[string]string{"d/x.css": "b", "i.html": "a"}, true},
		{"content differs", map[string]string{"i.html": "a"}, map[string]string{"i.html": "b"}, false},
		{"path differs", map[string]string{"i.html": "a"}, map[string]string{"j.html": "a"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			da, db := t.TempDir(), t.TempDir()
			writeTree(t, da, tt.a)
			writeTree(t, db, tt.b)
			ha, err := HashStaticTree(da)
			if err != nil {
				t.Fatal(err)
			}
			hb, err := HashStaticTree(db)
			if err != nil {
				t.Fatal(err)
			}
			if (ha.Digest == hb.Digest) != tt.wantEqual {
				t.Errorf("equal = %v, want %v", ha.Digest == hb.Digest, tt.wantEqual)
			}
		})
	}
}
