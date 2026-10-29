package alerting

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeCopySource []BackupCopyPending

func (f fakeCopySource) PendingBackupCopies(context.Context) ([]BackupCopyPending, error) {
	return f, nil
}

func TestEvaluateBackupCopyBehind(t *testing.T) {
	now := time.Now()
	src := fakeCopySource{{Resource: "pg", BackupID: "b1", FinishedAt: now.Add(-3 * time.Hour), LastError: "timeout"}}
	_, notice, err := EvaluateBackupCopyBehind(context.Background(), src, time.Hour, Rule{ID: "r", Kind: KindBackupCopyBehind}, now)
	if err != nil || !strings.Contains(notice, "pg backup b1") {
		t.Fatalf("notice = %q, err = %v", notice, err)
	}
	_, notice, _ = EvaluateBackupCopyBehind(context.Background(), src, 6*time.Hour, Rule{ID: "r", Kind: KindBackupCopyBehind}, now)
	if notice != "" {
		t.Fatalf("within limit should be quiet, got %q", notice)
	}
}
