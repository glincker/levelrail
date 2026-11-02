package backup

import (
	"context"
	"testing"
	"time"
)

func TestRetentionRuleDefaultWindow(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	hours := 168
	s := &Scheduler{Now: func() time.Time { return now }, DefaultRetentionHours: func(context.Context) int { return hours }}
	ctx := context.Background()

	if _, cut, ok := s.retentionRule(ctx, 0, 0); !ok || !cut.Equal(now.Add(-168*time.Hour)) {
		t.Fatalf("default window: ok=%v cut=%v", ok, cut)
	}
	if keep, cut, ok := s.retentionRule(ctx, 3, 0); !ok || keep != 3 || !cut.IsZero() {
		t.Fatalf("own count rule must win: keep=%d cut=%v ok=%v", keep, cut, ok)
	}
	if _, cut, ok := s.retentionRule(ctx, 0, 30); !ok || !cut.Equal(now.AddDate(0, 0, -30)) {
		t.Fatalf("own age rule must win: cut=%v ok=%v", cut, ok)
	}
	hours = 0
	if _, _, ok := s.retentionRule(ctx, 0, 0); ok {
		t.Fatal("0 hours keeps backups")
	}
}
