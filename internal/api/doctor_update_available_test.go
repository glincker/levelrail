package api

import (
	"context"
	"testing"

	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/version"
)

type stableSettings struct{}

func (stableSettings) GetUpdateSettings(context.Context) (store.UpdateSettings, error) {
	return store.UpdateSettings{Channel: "stable"}, nil
}

func (stableSettings) UpdateUpdateSettings(context.Context, store.UpdateSettings) error { return nil }

func TestDoctorCheckUpdateAvailable(t *testing.T) {
	setVersion(t, "v1.0.0")
	rt := &Router{updatesCache: newUpdatesCache(), updateSettings: stableSettings{}}
	if got := rt.doctorCheckUpdateAvailable(context.Background()); len(got) != 0 {
		t.Fatalf("no cached release must skip quietly, got %v", got)
	}
	rt.updatesCache.set(&githubRelease{TagName: "v1.2.0"})
	got := rt.doctorCheckUpdateAvailable(context.Background())
	if len(got) != 1 || got[0].Status != doctorStatusWarn || got[0].Fix == "" {
		t.Fatalf("want one warn with a fix, got %+v", got)
	}
	version.Version = "v1.2.0"
	got = rt.doctorCheckUpdateAvailable(context.Background())
	if len(got) != 1 || got[0].Status != doctorStatusOK {
		t.Fatalf("want ok when current, got %+v", got)
	}
}
