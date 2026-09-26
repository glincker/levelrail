package deploy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCheckOrderInterleavings(t *testing.T) {
	t0 := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	applied := store.DeployCursor{AppliedSequence: 7, CommitSHA: "bbbb", Before: "aaaa", CommitAt: t0}
	tests := []struct {
		name       string
		cursor     store.DeployCursor
		order      store.DeployOrder
		wantReason string
	}{
		{name: "first deploy ever", cursor: store.DeployCursor{}, order: store.DeployOrder{Sequence: 1, Automatic: true, CommitSHA: "aaaa"}},
		{name: "newer commit after older", cursor: applied, order: store.DeployOrder{Sequence: 8, Automatic: true, CommitSHA: "cccc", Before: "bbbb", CommitAt: t0.Add(time.Minute)}},
		{name: "older build finishes after newer trigger applied", cursor: applied, order: store.DeployOrder{Sequence: 6, Automatic: true, CommitSHA: "aaaa"}, wantReason: store.DeployReasonSuperseded},
		{name: "webhook for parent commit arrives late", cursor: applied, order: store.DeployOrder{Sequence: 9, Automatic: true, CommitSHA: "aaaa"}, wantReason: store.DeployReasonStale},
		{name: "webhook for older commit by timestamp arrives late", cursor: applied, order: store.DeployOrder{Sequence: 9, Automatic: true, CommitSHA: "zzzz", CommitAt: t0.Add(-time.Hour)}, wantReason: store.DeployReasonStale},
		{name: "redelivery of the deployed commit is allowed", cursor: applied, order: store.DeployOrder{Sequence: 9, Automatic: true, CommitSHA: "bbbb", CommitAt: t0}},
		{name: "unknown commit time is not judged stale", cursor: applied, order: store.DeployOrder{Sequence: 9, Automatic: true, CommitSHA: "dddd"}},
		{name: "manual redeploy of an old commit is exempt", cursor: applied, order: store.DeployOrder{Sequence: 3, CommitSHA: "aaaa"}},
		{name: "explicit rollback is exempt", cursor: applied, order: store.DeployOrder{Sequence: 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckOrder(tt.cursor, tt.order)
			if tt.wantReason == "" {
				if err != nil {
					t.Fatalf("want applied, got %v", err)
				}
				return
			}
			if !errors.Is(err, ErrSuperseded) {
				t.Fatalf("want superseded, got %v", err)
			}
			if r, _ := SupersededReason(err); r != tt.wantReason {
				t.Fatalf("reason = %q, want %q", r, tt.wantReason)
			}
		})
	}
}

// orderedFake applies CheckOrder against an in-memory cursor, mirroring
// store.SaveDesiredServiceOrdered's contract.
type orderedFake struct {
	cursor store.DeployCursor
	saved  []store.DesiredService
}

func (f *orderedFake) SaveDesiredServiceOrdered(_ context.Context, svc store.DesiredService, o store.DeployOrder, check store.DeployOrderCheck) error {
	if err := check(f.cursor, o); err != nil {
		return err
	}
	if o.Sequence > f.cursor.AppliedSequence {
		f.cursor.AppliedSequence = o.Sequence
	}
	if o.Automatic && o.CommitSHA != "" {
		f.cursor.CommitSHA, f.cursor.Before, f.cursor.CommitAt = o.CommitSHA, o.Before, o.CommitAt
	}
	f.saved = append(f.saved, svc)
	return nil
}

func TestPipelineRejectsSlowOlderBuild(t *testing.T) {
	ordered := &orderedFake{}
	p := New(&fakeBuilder{}, &fakeServiceStore{}, WithOrderedStore(ordered))
	deploy := func(seq int64, sha string) error {
		_, err := p.Deploy(context.Background(), Request{
			ServiceName: "web",
			Order:       &store.DeployOrder{Sequence: seq, Automatic: true, CommitSHA: sha},
			Service:     spec.Service{Port: 80, Build: spec.Build{Type: spec.BuildImage, Image: "web:" + sha}},
		}, nil)
		return err
	}
	// A triggered first (seq 1), B second (seq 2); B's deploy lands first.
	if err := deploy(2, "bbbb"); err != nil {
		t.Fatal(err)
	}
	if err := deploy(1, "aaaa"); !errors.Is(err, ErrSuperseded) {
		t.Fatalf("slow older deploy err = %v, want superseded", err)
	}
	if len(ordered.saved) != 1 || ordered.saved[0].Image != "web:bbbb" {
		t.Fatalf("saved = %+v", ordered.saved)
	}
}
