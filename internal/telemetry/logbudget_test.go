package telemetry

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func seedBudgetLogs(t *testing.T, db *DB, id string, n int, size int, start time.Time) {
	t.Helper()
	entries := make([]LogEntry, n)
	for i := range entries {
		msg := fmt.Sprintf("%05d-", i) + strings.Repeat("x", size-6)
		entries[i] = LogEntry{ResourceID: id, Stream: "stdout", Timestamp: start.Add(time.Duration(i) * time.Second), Message: msg}
	}
	if err := db.WriteLogBatch(context.Background(), entries); err != nil {
		t.Fatalf("WriteLogBatch: %v", err)
	}
}

func TestEnforceLogBudgets(t *testing.T) {
	tests := []struct {
		name        string
		defaultMax  int64
		overrideA   int64
		wantALines  int
		wantBLines  int
		wantTrimmed []string
	}{
		{"unlimited keeps everything", 0, 0, 100, 100, nil},
		{"override trims only that app", 0, 5000, 50, 100, []string{"service:a"}},
		{"default trims both over budget", 5000, 0, 50, 50, []string{"service:a", "service:b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			ctx := context.Background()
			base := time.Now().Add(-time.Hour)
			seedBudgetLogs(t, db, "service:a", 100, 100, base)
			seedBudgetLogs(t, db, "service:b", 100, 100, base)
			if tt.overrideA > 0 {
				if err := db.SetLogBudget(ctx, "service:a", tt.overrideA); err != nil {
					t.Fatal(err)
				}
			}
			trims, err := db.EnforceLogBudgets(ctx, tt.defaultMax)
			if err != nil {
				t.Fatalf("EnforceLogBudgets: %v", err)
			}
			var got []string
			for _, tr := range trims {
				got = append(got, tr.ResourceID)
			}
			if strings.Join(got, ",") != strings.Join(tt.wantTrimmed, ",") {
				t.Errorf("trimmed = %v, want %v", got, tt.wantTrimmed)
			}
			usage, err := db.LogUsageAll(ctx, time.Now(), tt.defaultMax)
			if err != nil {
				t.Fatal(err)
			}
			lines := map[string]int64{}
			for _, u := range usage {
				lines[u.ResourceID] = u.Lines
			}
			if lines["service:a"] != int64(tt.wantALines) || lines["service:b"] != int64(tt.wantBLines) {
				t.Errorf("lines a=%d b=%d, want %d %d", lines["service:a"], lines["service:b"], tt.wantALines, tt.wantBLines)
			}
		})
	}
}

func TestEnforceLogBudgetsDropsOldestAndKeepsFTS(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	base := time.Now().Add(-time.Hour)
	seedBudgetLogs(t, db, "service:a", 100, 100, base)
	if err := db.SetLogBudget(ctx, "service:a", 5000); err != nil {
		t.Fatal(err)
	}
	if _, err := db.EnforceLogBudgets(ctx, 0); err != nil {
		t.Fatal(err)
	}
	got, err := db.QueryLogs(ctx, "service:a", base.Add(-time.Hour), time.Now(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || !strings.HasPrefix(got[len(got)-1].Message, "00099-") {
		t.Fatalf("newest line missing after trim: %d rows", len(got))
	}
	if strings.HasPrefix(got[0].Message, "00000-") {
		t.Errorf("oldest line survived trim")
	}
	hits, err := db.QueryLogs(ctx, "service:a", base.Add(-time.Hour), time.Now(), "00000")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("FTS still matches trimmed row: %d", len(hits))
	}
	usage, _ := db.LogUsageAll(ctx, time.Now(), 0)
	if usage[0].TrimmedLines != 100-usage[0].Lines {
		t.Errorf("trimmed lines = %d, want %d", usage[0].TrimmedLines, 100-usage[0].Lines)
	}
}

func TestEnforceLogBudgetsBoundedPerSweep(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	rows := trimBatchRows*trimMaxBatches + 500
	seedBudgetLogs(t, db, "service:a", rows, 50, time.Now().Add(-time.Hour))
	if err := db.SetLogBudget(ctx, "service:a", 1000); err != nil {
		t.Fatal(err)
	}
	trims, err := db.EnforceLogBudgets(ctx, 0)
	if err != nil || len(trims) != 1 {
		t.Fatalf("trims=%v err=%v", trims, err)
	}
	if trims[0].Lines != trimBatchRows*trimMaxBatches {
		t.Errorf("dropped %d, want bounded %d", trims[0].Lines, trimBatchRows*trimMaxBatches)
	}
}

func TestForecastDays(t *testing.T) {
	day := 60.0 * 24
	tests := []struct {
		name      string
		used, max int64
		rate      float64
		want      *float64
	}{
		{"unlimited", 10, 0, 5, nil},
		{"idle", 10, 100, 0, nil},
		{"over budget", 200, 100, 5, ptr(0)},
		{"two days", 0, int64(2 * day * 10), 10, ptr(2)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ForecastDays(tt.used, tt.max, tt.rate)
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("got %v want %v", got, tt.want)
			}
		})
	}
}

func ptr(f float64) *float64 { return &f }

func TestDefaultLogBudgetBytes(t *testing.T) {
	t.Setenv("APP_LOG_BUDGET_MB", "2")
	if got := DefaultLogBudgetBytes(); got != 2<<20 {
		t.Errorf("got %d", got)
	}
	t.Setenv("APP_LOG_BUDGET_MB", "0")
	if got := DefaultLogBudgetBytes(); got != 0 {
		t.Errorf("got %d", got)
	}
}
