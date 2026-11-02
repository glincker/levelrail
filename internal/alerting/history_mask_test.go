package alerting

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

func TestRecordHistoryMasksRegisteredSecrets(t *testing.T) {
	const secret = "alertcanary-52d1f0aa-token" //nolint:gosec // synthetic test value
	secretmask.Add(secret)
	db := newTestDB(t)
	ctx := context.Background()
	err := db.RecordHistory(ctx, HistoryEntry{
		RuleID: "r1", RuleName: "cpu", Event: EventFired, Outcome: OutcomeFailed, Severity: SeverityWarning,
		Detail: "log line: password=" + secret + " seen", Error: "post https://hooks.example/" + secret + ": 500",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListHistory(ctx, HistoryFilter{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("history = %v, %v", rows, err)
	}
	for _, f := range []string{rows[0].Detail, rows[0].Error} {
		if strings.Contains(f, secret) {
			t.Errorf("secret survived in %q", f)
		}
	}
	if !strings.Contains(rows[0].Detail, "log line: password=") || !strings.Contains(rows[0].Error, ": 500") {
		t.Errorf("surrounding text lost: %q / %q", rows[0].Detail, rows[0].Error)
	}
}
