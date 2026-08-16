package telemetry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type fakeStatsSource struct {
	stats  map[string]docker.ContainerStats
	errFor map[string]error
	calls  []string
}

func (f *fakeStatsSource) Stats(_ context.Context, containerID string) (docker.ContainerStats, error) {
	f.calls = append(f.calls, containerID)
	if err, ok := f.errFor[containerID]; ok {
		return docker.ContainerStats{}, err
	}
	return f.stats[containerID], nil
}

func TestCollectOnce_WritesSamplesForEveryTarget(t *testing.T) {
	db := newTestDB(t)
	source := &fakeStatsSource{
		stats: map[string]docker.ContainerStats{
			"c1": {CPUPercent: 10, MemoryUsageBytes: 100, MemoryLimitBytes: 1000, NetworkRxBytes: 1, NetworkTxBytes: 2, DiskReadBytes: 3, DiskWriteBytes: 4},
			"c2": {CPUPercent: 20, MemoryUsageBytes: 200, MemoryLimitBytes: 2000, NetworkRxBytes: 5, NetworkTxBytes: 6, DiskReadBytes: 7, DiskWriteBytes: 8},
		},
	}
	c := NewCollector(source, db, time.Second, nil)

	err := c.CollectOnce(context.Background(), []Target{
		{ResourceID: "service:web", ContainerID: "c1"},
		{ResourceID: "service:worker", ContainerID: "c2"},
	})
	if err != nil {
		t.Fatalf("CollectOnce() error = %v", err)
	}

	got, err := db.Query(context.Background(), "service:web", "cpu_percent", time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(got) != 1 || got[0].Value != 10 {
		t.Errorf("service:web cpu_percent = %+v, want one sample with value 10", got)
	}

	gotMem, err := db.Query(context.Background(), "service:worker", "memory_usage_bytes", time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(gotMem) != 1 || gotMem[0].Value != 200 {
		t.Errorf("service:worker memory_usage_bytes = %+v, want one sample with value 200", gotMem)
	}
}

func TestCollectOnce_OneTargetFails_OthersStillWritten(t *testing.T) {
	db := newTestDB(t)
	source := &fakeStatsSource{
		stats:  map[string]docker.ContainerStats{"c-ok": {CPUPercent: 42}},
		errFor: map[string]error{"c-gone": errors.New("no such container")},
	}
	c := NewCollector(source, db, time.Second, nil)

	err := c.CollectOnce(context.Background(), []Target{
		{ResourceID: "service:gone", ContainerID: "c-gone"},
		{ResourceID: "service:ok", ContainerID: "c-ok"},
	})
	if err == nil {
		t.Fatal("CollectOnce() error = nil, want the c-gone failure surfaced")
	}

	got, qerr := db.Query(context.Background(), "service:ok", "cpu_percent", time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if qerr != nil {
		t.Fatalf("Query() error = %v", qerr)
	}
	if len(got) != 1 || got[0].Value != 42 {
		t.Errorf("service:ok's sample = %+v, want it written despite service:gone's failure", got)
	}

	goneSamples, qerr := db.Query(context.Background(), "service:gone", "cpu_percent", time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if qerr != nil {
		t.Fatalf("Query() error = %v", qerr)
	}
	if len(goneSamples) != 0 {
		t.Errorf("service:gone samples = %+v, want none written for a target whose Stats call failed", goneSamples)
	}
}

func TestCollectOnce_EmptyTargets_NoOp(t *testing.T) {
	db := newTestDB(t)
	c := NewCollector(&fakeStatsSource{}, db, time.Second, nil)
	if err := c.CollectOnce(context.Background(), nil); err != nil {
		t.Errorf("CollectOnce(nil) error = %v, want nil", err)
	}
}

func TestRun_CallsTargetsFuncFreshEveryTick(t *testing.T) {
	if testing.Short() {
		t.Skip("tight 10ms tick timing is flaky under load on shared CI runners, covered by the nightly full suite")
	}
	db := newTestDB(t)
	source := &fakeStatsSource{stats: map[string]docker.ContainerStats{"c1": {CPUPercent: 1}}}
	c := NewCollector(source, db, 10*time.Millisecond, nil)

	var callCount int
	targetsFunc := func(context.Context) ([]Target, error) {
		callCount++
		return []Target{{ResourceID: "service:web", ContainerID: "c1"}}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Millisecond)
	defer cancel()

	err := c.Run(ctx, targetsFunc)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context.DeadlineExceeded", err)
	}
	if callCount < 3 {
		t.Errorf("targetsFunc called %d times in ~55ms at a 10ms interval, want at least 3 (fresh lookup every tick)", callCount)
	}
}

func TestRun_TargetsFuncError_ContinuesToNextTick(t *testing.T) {
	db := newTestDB(t)
	c := NewCollector(&fakeStatsSource{}, db, 10*time.Millisecond, nil)

	var callCount int
	targetsFunc := func(context.Context) ([]Target, error) {
		callCount++
		return nil, errors.New("store unreachable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
	defer cancel()

	err := c.Run(ctx, targetsFunc)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context.DeadlineExceeded (a targetsFunc error must not stop the loop)", err)
	}
	if callCount < 2 {
		t.Errorf("targetsFunc called %d times, want at least 2: a failing tick must not stop the collector from trying again", callCount)
	}
}
