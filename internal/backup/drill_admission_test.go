package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDrillReserveRefusesDuplicateAndOverCap(t *testing.T) {
	r := &DrillRunner{MaxConcurrentFn: func() int { return 2 }}
	relA, err := r.Reserve("a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reserve("a"); !errors.Is(err, ErrDrillBusy) {
		t.Fatalf("duplicate = %v, want ErrDrillBusy", err)
	}
	if _, err := r.Reserve("b"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reserve("c"); !errors.Is(err, ErrDrillCapacity) {
		t.Fatalf("over cap = %v, want ErrDrillCapacity", err)
	}
	relA()
	if _, err := r.Reserve("c"); err != nil {
		t.Fatalf("after release = %v", err)
	}
}

func TestRunDrillTimesOutAndFinishesRow(t *testing.T) {
	hist, objs := newMemHistory(), newMemObjects()
	hist.rows["bkh_1"] = store.BackupHistory{ID: "bkh_1", ResourceKind: store.BackupResourceKindDatabase, DatabaseName: "main", TargetID: "bkt_test", ObjectKey: "k", Status: store.BackupStatusSucceeded}
	r := &DrillRunner{
		Store: hist, Secrets: newTestSecrets(), Downloader: objs, Prober: blockingProber{},
		TimeoutFn: func() time.Duration { return 20 * time.Millisecond },
	}
	d, err := r.RunDrill(context.Background(), "bkd_1", "bkh_1", store.DrillTriggerManual)
	if err == nil || d.Status != store.DrillStatusFailed {
		t.Fatalf("want timed out failure, got %v %+v", err, d)
	}
	if hist.drills["bkd_1"].FinishedAt == "" {
		t.Fatal("drill row was not finished after the timeout")
	}
	if _, err := r.Reserve("bkh_1"); err != nil {
		t.Fatalf("reservation leaked after timeout: %v", err)
	}
}

type blockingProber struct{}

func (blockingProber) Head(ctx context.Context, _ Destination, _ string) (ObjectInfo, error) {
	<-ctx.Done()
	return ObjectInfo{}, ctx.Err()
}

func TestDrillHelperIsolation(t *testing.T) {
	r := &DrillRunner{}
	rt := &removeTrackingRuntime{fakeExecRuntime: &fakeExecRuntime{}}
	var got docker.ContainerSpec
	hr := &specCapture{removeTrackingRuntime: rt, spec: &got}
	ctx := withHelperHardening(context.Background(), r.hardenHelper)
	if _, err := createVolumeHelper(ctx, hr, "v", "volrestore", false); err != nil {
		t.Fatal(err)
	}
	if got.NetworkMode != "none" || got.PidsLimit <= 0 || got.Resources == nil || len(got.SecurityOpt) != 1 {
		t.Fatalf("drill helper not isolated: %+v", got)
	}
	var plain docker.ContainerSpec
	if _, err := createVolumeHelper(context.Background(), &specCapture{removeTrackingRuntime: rt, spec: &plain}, "v", "volrestore", false); err != nil {
		t.Fatal(err)
	}
	if plain.NetworkMode != "" || plain.Resources != nil || len(plain.SecurityOpt) != 0 {
		t.Fatalf("normal helper changed: %+v", plain)
	}
}

type specCapture struct {
	*removeTrackingRuntime
	spec *docker.ContainerSpec
}

func (s *specCapture) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	*s.spec = spec
	return "h-1", nil
}
func (s *specCapture) Start(context.Context, string) error { return nil }

func TestManualDrillsCannotTakeTheSchedulerSlot(t *testing.T) {
	r := &DrillRunner{MaxConcurrentFn: func() int { return 2 }, ReservedScheduledFn: func() int { return 1 }}
	if _, err := r.ReserveManual("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReserveManual("b"); !errors.Is(err, ErrDrillCapacity) {
		t.Fatalf("second manual = %v, want ErrDrillCapacity", err)
	}
	if _, err := r.Reserve("c"); err != nil {
		t.Fatalf("scheduled drill refused: %v", err)
	}
}
