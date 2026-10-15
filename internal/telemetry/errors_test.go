package telemetry

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func openErrDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestErrorIssueRoundTripRegressionAndRing(t *testing.T) {
	db := openErrDB(t)
	ctx := context.Background()
	ev := func(n int64) ErrorEvent {
		return ErrorEvent{ResourceID: "service:web", Fingerprint: "fp1", Title: "Boom", Level: "error",
			EventID: "e", TimestampNs: n, Release: "r1", DeployID: "dep1", Raw: "{}"}
	}
	for i := int64(1); i <= 5; i++ {
		if _, err := db.WriteErrorEvent(ctx, ev(i), 3); err != nil {
			t.Fatal(err)
		}
	}
	issue, samples, err := db.GetErrorIssue(ctx, "service:web", "fp1")
	if err != nil {
		t.Fatal(err)
	}
	if issue.Count != 5 || issue.FirstSeenNs != 1 || issue.LastSeenNs != 5 || issue.LastDeployID != "dep1" {
		t.Fatalf("issue = %+v", issue)
	}
	if len(samples) != 3 || samples[0].TimestampNs != 5 || samples[2].TimestampNs != 3 {
		t.Fatalf("samples = %+v, want the newest 3", samples)
	}

	if err := db.SetErrorIssueStatus(ctx, "service:web", "fp1", ErrorStatusResolved); err != nil {
		t.Fatal(err)
	}
	reg, err := db.WriteErrorEvent(ctx, ev(6), 3)
	if err != nil || !reg {
		t.Fatalf("regressed = %v err = %v, want true", reg, err)
	}
	issue, _, _ = db.GetErrorIssue(ctx, "service:web", "fp1")
	if issue.Status != ErrorStatusOpen || !issue.Regressed {
		t.Fatalf("after regression = %+v", issue)
	}

	if err := db.SetErrorIssueStatus(ctx, "service:web", "fp1", ErrorStatusIgnored); err != nil {
		t.Fatal(err)
	}
	if reg, _ := db.WriteErrorEvent(ctx, ev(7), 3); reg {
		t.Fatal("an ignored issue must not report a regression")
	}
	issue, _, _ = db.GetErrorIssue(ctx, "service:web", "fp1")
	if issue.Status != ErrorStatusIgnored {
		t.Fatalf("status = %s, want ignored", issue.Status)
	}

	list, err := db.ListErrorIssues(ctx, ErrorFilter{ResourceID: "service:web", Status: ErrorStatusOpen})
	if err != nil || len(list) != 0 {
		t.Fatalf("open list = %+v err=%v", list, err)
	}
	if err := db.SetErrorIssueStatus(ctx, "service:web", "nope", ErrorStatusOpen); !errors.Is(err, ErrNoErrorIssue) {
		t.Fatalf("missing issue err = %v", err)
	}
	n, err := db.RetainErrors(ctx, time.Unix(0, 100))
	if err != nil || n != 1 {
		t.Fatalf("retain = %d err=%v, want 1 issue deleted", n, err)
	}
}
