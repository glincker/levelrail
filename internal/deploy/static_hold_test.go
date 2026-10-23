package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeStaticLocks struct {
	fakeLocks
	sites map[string]store.StaticSite
}

func (f *fakeStaticLocks) SaveHeldStaticBuild(_ context.Context, id string, site store.StaticSite, _ time.Time) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	if f.sites == nil {
		f.sites = map[string]store.StaticSite{}
	}
	f.sites[id] = site
	return nil
}

func TestPipeline_DeployStatic_PublishLock(t *testing.T) {
	locked := &store.PublishLock{ServiceName: "docs"}
	tests := []struct {
		name      string
		locks     PublishLocks
		wantErr   bool
		wantLive  int
		wantHeld  int
		wantStage bool
	}{
		{name: "unlocked goes live", locks: &fakeStaticLocks{}, wantLive: 1, wantStage: true},
		{name: "locked is staged and held", locks: &fakeStaticLocks{fakeLocks: fakeLocks{lock: locked}}, wantHeld: 1, wantStage: true},
		{name: "lookup failure fails closed", locks: &fakeStaticLocks{fakeLocks: fakeLocks{lock: locked, getErr: errors.New("db down")}}, wantErr: true},
		{name: "hold save failure leaves live untouched", locks: &fakeStaticLocks{fakeLocks: fakeLocks{lock: locked, saveErr: errors.New("disk full")}}, wantErr: true},
		{name: "store that cannot hold static builds fails closed", locks: &fakeLocks{lock: locked}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			staticStore := &fakeStaticSiteStore{}
			rootDir := t.TempDir()
			p := New(&fakeBuilder{}, &fakeServiceStore{}, WithStaticSiteStore(staticStore), WithStaticRootDir(rootDir), WithPublishLocks(tt.locks))
			sourceDir := t.TempDir()
			writeTree(t, sourceDir, map[string]string{"dist/index.html": "<h1>hi</h1>", "dist/_redirects": "/a /b 301\n"})

			dest, err := p.Deploy(context.Background(), Request{
				ServiceName: "docs", Service: staticService("docs.example.com"), SourceDir: sourceDir, CommitSHA: "abc1234", AttemptID: "dpl_1",
			}, nil)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Deploy() error = %v, wantErr %v", err, tt.wantErr)
			}
			if staticStore.saveCalls != tt.wantLive {
				t.Errorf("live pointer saves = %d, want %d", staticStore.saveCalls, tt.wantLive)
			}
			if sl, ok := tt.locks.(*fakeStaticLocks); ok && len(sl.sites) != tt.wantHeld {
				t.Fatalf("held static builds = %d, want %d", len(sl.sites), tt.wantHeld)
			}
			if tt.wantStage {
				if _, err := os.Stat(filepath.Join(dest, "index.html")); err != nil {
					t.Errorf("staged tree missing: %v", err)
				}
			}
			if tt.wantHeld == 1 {
				site := tt.locks.(*fakeStaticLocks).sites["dpl_1"]
				if site.RootDir != dest || site.Name != "docs" {
					t.Errorf("held site = %+v, want root %q", site, dest)
				}
				if _, err := os.Stat(filepath.Join(dest, "_redirects")); err == nil {
					t.Error("routing source file should be consumed from the held tree")
				}
			}
		})
	}
}

func TestPipeline_DeployStatic_RedeployWhileLockedConverges(t *testing.T) {
	locks := &fakeStaticLocks{fakeLocks: fakeLocks{lock: &store.PublishLock{ServiceName: "docs"}}}
	staticStore := &fakeStaticSiteStore{}
	p := New(&fakeBuilder{}, &fakeServiceStore{}, WithStaticSiteStore(staticStore), WithStaticRootDir(t.TempDir()), WithPublishLocks(locks))
	sourceDir := t.TempDir()
	writeTree(t, sourceDir, map[string]string{"dist/index.html": "v1"})
	req := Request{ServiceName: "docs", Service: staticService(), SourceDir: sourceDir, CommitSHA: "abc", AttemptID: "dpl_1"}
	for i := 0; i < 2; i++ {
		if _, err := p.Deploy(context.Background(), req, nil); err != nil {
			t.Fatalf("deploy %d: %v", i, err)
		}
	}
	if len(locks.sites) != 1 || staticStore.saveCalls != 0 {
		t.Fatalf("held = %d, live saves = %d, want one held and no live save", len(locks.sites), staticStore.saveCalls)
	}
}

func TestImageTrigger_PublishLock(t *testing.T) {
	locked := &store.PublishLock{ServiceName: "web"}
	tests := []struct {
		name      string
		locks     *fakeLocks
		wantErr   bool
		wantLive  int
		wantHeld  int
		wantNudge int
	}{
		{name: "unlocked goes live", locks: &fakeLocks{}, wantLive: 1, wantNudge: 1},
		{name: "locked is held and not nudged", locks: &fakeLocks{lock: locked}, wantHeld: 1},
		{name: "lookup failure fails closed", locks: &fakeLocks{lock: locked, getErr: errors.New("db down")}, wantErr: true},
		{name: "hold save failure fails closed", locks: &fakeLocks{lock: locked, saveErr: errors.New("disk full")}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &fakeImageDeployStore{}
			n := &fakeNudger{}
			_, err := ImageTrigger{Store: st, Nudger: n, Locks: tt.locks}.Deploy(context.Background(),
				store.DesiredService{Name: "web", Image: "web:live"}, "web:new")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Deploy() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(st.saved) != tt.wantLive || len(tt.locks.held) != tt.wantHeld || n.calls != tt.wantNudge {
				t.Fatalf("live = %d, held = %d, nudges = %d; want %d, %d, %d", len(st.saved), len(tt.locks.held), n.calls, tt.wantLive, tt.wantHeld, tt.wantNudge)
			}
			if tt.wantHeld == 1 {
				if len(st.attempts) != 1 {
					t.Fatalf("attempts = %d, want 1", len(st.attempts))
				}
				if held, ok := tt.locks.held[st.attempts[0].ID]; !ok || held.Image != "web:new" {
					t.Errorf("held build must be keyed by the attempt id with the new image, got %+v", tt.locks.held)
				}
			}
		})
	}
}
