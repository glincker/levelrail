package alerting

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeBackupSizes []BackupSizeSeries

func (f fakeBackupSizes) RecentBackupSizes(context.Context) ([]BackupSizeSeries, error) {
	return f, nil
}

func TestEvaluateBackupSizeAnomaly(t *testing.T) {
	now := time.Now()
	r := Rule{ID: "r", Kind: KindBackupSizeAnomaly}
	pol := BackupSizePolicy{Percent: 25, Baseline: 3}
	src := fakeBackupSizes{
		{Resource: "pg-main", Sizes: []int64{10, 1000, 1100, 900}},
		{Resource: "pg-ok", Sizes: []int64{800, 1000, 1100, 900}},
		{Resource: "young", Sizes: []int64{1, 1000}},
	}
	_, notice, err := EvaluateBackupSizeAnomaly(context.Background(), src, pol, r, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(notice, "pg-main") || strings.Contains(notice, "pg-ok") || strings.Contains(notice, "young") {
		t.Fatalf("notice = %q", notice)
	}
	if _, notice, _ = EvaluateBackupSizeAnomaly(context.Background(), src, BackupSizePolicy{}, r, now); notice != "" {
		t.Fatalf("disabled policy fired: %q", notice)
	}
}
