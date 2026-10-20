package alerting

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeExportHealth []ExportHealth

func (f fakeExportHealth) ExportHealth(context.Context) ([]ExportHealth, error) { return f, nil }

func TestEvaluateExportBehind(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		dest     ExportHealth
		wantFire bool
	}{
		{"healthy", ExportHealth{Name: "a", Enabled: true, LastSuccessAt: now.Add(-time.Minute)}, false},
		{"failing but recent success", ExportHealth{Name: "a", Enabled: true, LastError: "boom", LastSuccessAt: now.Add(-time.Minute)}, false},
		{"failing past limit", ExportHealth{Name: "a", Enabled: true, LastError: "boom", LastSuccessAt: now.Add(-time.Hour)}, true},
		{"never succeeded, old", ExportHealth{Name: "a", Enabled: true, LastError: "boom", CreatedAt: now.Add(-time.Hour)}, true},
		{"breaker open", ExportHealth{Name: "a", Enabled: true, CircuitOpen: true, LastSuccessAt: now.Add(-time.Hour)}, true},
		{"disabled", ExportHealth{Name: "a", Enabled: false, LastError: "boom", LastSuccessAt: now.Add(-time.Hour)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, notice, err := EvaluateExportBehind(context.Background(), fakeExportHealth{tt.dest}, Rule{ID: "r1", Kind: KindExportBehind}, now)
			if err != nil {
				t.Fatal(err)
			}
			if fired := notice != ""; fired != tt.wantFire {
				t.Fatalf("notice = %q, wantFire %t", notice, tt.wantFire)
			}
			if tt.wantFire && !strings.Contains(notice, "a:") {
				t.Fatalf("notice does not name the destination: %q", notice)
			}
		})
	}
}
