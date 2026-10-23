package deploy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeLocks struct {
	lock    *store.PublishLock
	getErr  error
	saveErr error
	held    map[string]store.DesiredService
}

func (f *fakeLocks) GetPublishLock(context.Context, string) (*store.PublishLock, error) {
	return f.lock, f.getErr
}

func (f *fakeLocks) SaveHeldBuild(_ context.Context, id string, svc store.DesiredService, _ time.Time) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	if f.held == nil {
		f.held = map[string]store.DesiredService{}
	}
	f.held[id] = svc
	return nil
}

func TestPipeline_Deploy_PublishLock(t *testing.T) {
	locked := &store.PublishLock{ServiceName: "web", LockedBy: "gagan"}
	tests := []struct {
		name        string
		locks       *fakeLocks
		wantErr     bool
		wantSaved   int
		wantHeldIDs int
	}{
		{name: "unlocked app goes live", locks: &fakeLocks{}, wantSaved: 1},
		{name: "locked app builds but is held", locks: &fakeLocks{lock: locked}, wantHeldIDs: 1},
		{name: "lock lookup failure fails closed", locks: &fakeLocks{lock: locked, getErr: errors.New("db down")}, wantErr: true},
		{name: "held build save failure leaves live untouched", locks: &fakeLocks{lock: locked, saveErr: errors.New("disk full")}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svcStore := &fakeServiceStore{}
			p := New(&fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc"}}, svcStore, WithPublishLocks(tt.locks))
			tag, err := p.Deploy(context.Background(), Request{
				ServiceName: "web", Service: dockerfileService(), SourceDir: "/repo", CommitSHA: "abc", ImageRepo: "levelrail/web", AttemptID: "dpl_1",
			}, nil)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Deploy() error = %v, wantErr %v", err, tt.wantErr)
			}
			if svcStore.saveCalls != tt.wantSaved {
				t.Errorf("live saves = %d, want %d", svcStore.saveCalls, tt.wantSaved)
			}
			if len(tt.locks.held) != tt.wantHeldIDs {
				t.Fatalf("held builds = %d, want %d", len(tt.locks.held), tt.wantHeldIDs)
			}
			if tt.wantHeldIDs == 1 {
				if got := tt.locks.held["dpl_1"].Image; got != "levelrail/web:abc" || tag != got {
					t.Errorf("held image = %q, tag = %q", got, tag)
				}
			}
		})
	}
}
