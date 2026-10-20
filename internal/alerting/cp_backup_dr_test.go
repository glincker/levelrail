package alerting

import (
	"strings"
	"testing"
	"time"
)

type fakeCPDR struct {
	fakeCPBackups
	problem string
}

func (f fakeCPDR) DRProblem(time.Time) string { return f.problem }

func TestEvaluateControlPlaneBackupStale_DRProblem(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		src        fakeCPDR
		wantFiring bool
		wantNotice string
	}{
		{name: "healthy", src: fakeCPDR{fakeCPBackups: fakeCPBackups{newest: now.Add(-time.Hour), ok: true}}},
		{name: "fresh snapshot but failed drill", src: fakeCPDR{fakeCPBackups: fakeCPBackups{newest: now.Add(-time.Hour), ok: true}, problem: "restore drill failed: boom"}, wantFiring: true, wantNotice: "drill failed"},
		{name: "never succeeded but failing", src: fakeCPDR{problem: "off-box backup failed: denied"}, wantFiring: true, wantNotice: "off-box backup failed"},
		{name: "no snapshot and no problem is quiet", src: fakeCPDR{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			next, notice, err := EvaluateControlPlaneBackupStale(tc.src, Rule{ID: "r1"}, 0, now)
			if err != nil {
				t.Fatal(err)
			}
			if next.Firing != tc.wantFiring {
				t.Errorf("Firing = %v, want %v", next.Firing, tc.wantFiring)
			}
			if !strings.Contains(notice, tc.wantNotice) || (tc.wantNotice == "" && notice != "") {
				t.Errorf("notice = %q, want it to contain %q", notice, tc.wantNotice)
			}
		})
	}
}
