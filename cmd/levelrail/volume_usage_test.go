package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type fakeVolDocker struct{ vols []docker.NamedVolume }

func (f fakeVolDocker) ListNamedVolumes(context.Context) ([]docker.NamedVolume, error) {
	return f.vols, nil
}

type fakeVolSvcs struct{ svcs []store.DesiredService }

func (f fakeVolSvcs) ListDesiredServices(context.Context) ([]store.DesiredService, error) {
	return f.svcs, nil
}

type fakeVolWriter struct{ got []telemetry.Sample }

func (f *fakeVolWriter) WriteSamples(_ context.Context, s []telemetry.Sample) error {
	f.got = append(f.got, s...)
	return nil
}

func TestVolumeUsageSample(t *testing.T) {
	w := &fakeVolWriter{}
	v := &volumeUsage{
		docker: fakeVolDocker{vols: []docker.NamedVolume{{Name: "data", SizeBytes: 50}, {Name: "nosize", SizeBytes: -1}}},
		services: fakeVolSvcs{svcs: []store.DesiredService{
			{Name: "web", Volumes: []store.ServiceVolume{{Name: "data"}, {Name: "nosize"}, {Name: "gone"}}},
		}},
		writer: w,
		total:  func() int64 { return 200 },
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if err := v.sample(context.Background(), time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	if len(w.got) != 2 {
		t.Fatalf("want 2 samples, got %d", len(w.got))
	}
	if w.got[0].ResourceID != "volume:web/data" || w.got[0].Value != 50 || w.got[1].Value != 200 {
		t.Fatalf("unexpected samples: %+v", w.got)
	}
}

func TestVolumeUsageSampleSkipsUnknownCapacity(t *testing.T) {
	w := &fakeVolWriter{}
	v := &volumeUsage{docker: fakeVolDocker{}, services: fakeVolSvcs{}, writer: w, total: func() int64 { return 0 }}
	if err := v.sample(context.Background(), time.Now()); err != nil || len(w.got) != 0 {
		t.Fatalf("want no writes, got %v %v", err, w.got)
	}
}
