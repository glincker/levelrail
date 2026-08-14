package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestRecordDeploy_WritesOneSample(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	at := time.Unix(1_700_000_000, 0).UTC()

	if err := db.RecordDeploy(ctx, "web", at); err != nil {
		t.Fatalf("RecordDeploy() error = %v", err)
	}

	got, err := db.Query(ctx, "service:web", MetricDeployCount, at.Add(-time.Minute), at.Add(time.Minute))
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Query() = %d samples, want 1", len(got))
	}
	if got[0].Value != 1 {
		t.Errorf("Query()[0].Value = %v, want 1", got[0].Value)
	}
	if !got[0].Timestamp.Equal(at) {
		t.Errorf("Query()[0].Timestamp = %v, want %v", got[0].Timestamp, at)
	}
}

func TestRecordDeploy_MultipleDeploys_EachCounted(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	if err := db.RecordDeploy(ctx, "web", base); err != nil {
		t.Fatalf("first RecordDeploy() error = %v", err)
	}
	if err := db.RecordDeploy(ctx, "web", base.Add(time.Hour)); err != nil {
		t.Fatalf("second RecordDeploy() error = %v", err)
	}

	got, err := db.Query(ctx, "service:web", MetricDeployCount, base.Add(-time.Minute), base.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Query() = %d samples, want 2 (one per deploy)", len(got))
	}

	bucketed := Aggregate(got, base.Add(-time.Minute), 3*time.Hour)
	if len(bucketed) != 1 || bucketed[0].Count != 2 {
		t.Errorf("Aggregate() = %+v, want a single bucket with Count 2 (deploy frequency for this range)", bucketed)
	}
}

func TestRecordDeploy_DifferentServices_DoNotLeak(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	at := time.Unix(1_700_000_000, 0).UTC()

	if err := db.RecordDeploy(ctx, "web", at); err != nil {
		t.Fatalf("RecordDeploy(web) error = %v", err)
	}
	if err := db.RecordDeploy(ctx, "worker", at); err != nil {
		t.Fatalf("RecordDeploy(worker) error = %v", err)
	}

	got, err := db.Query(ctx, "service:web", MetricDeployCount, at.Add(-time.Minute), at.Add(time.Minute))
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(got) != 1 {
		t.Errorf("Query(service:web) = %d samples, want 1 (worker's deploy must not leak in)", len(got))
	}
}

func TestRecordBuildDuration_WritesOneSample(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	at := time.Unix(1_700_000_000, 0).UTC()

	if err := db.RecordBuildDuration(ctx, "web", 42*time.Second, at); err != nil {
		t.Fatalf("RecordBuildDuration() error = %v", err)
	}

	got, err := db.Query(ctx, "service:web", MetricBuildDuration, at.Add(-time.Minute), at.Add(time.Minute))
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Query() = %d samples, want 1", len(got))
	}
	if got[0].Value != 42 {
		t.Errorf("Query()[0].Value = %v, want 42 (seconds)", got[0].Value)
	}
}
