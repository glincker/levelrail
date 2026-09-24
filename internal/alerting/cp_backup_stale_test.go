package alerting

import (
	"errors"
	"testing"
	"time"
)

type fakeCPBackups struct {
	newest time.Time
	ok     bool
	err    error
}

func (f fakeCPBackups) Newest() (time.Time, bool, error) { return f.newest, f.ok, f.err }

func TestEvaluateControlPlaneBackupStale(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		src        fakeCPBackups
		forDur     time.Duration
		wantFiring bool
		wantErr    bool
	}{
		{name: "fresh snapshot", src: fakeCPBackups{newest: now.Add(-time.Hour), ok: true}},
		{name: "older than default limit", src: fakeCPBackups{newest: now.Add(-4 * 24 * time.Hour), ok: true}, wantFiring: true},
		{name: "rule limit overrides default", src: fakeCPBackups{newest: now.Add(-5 * time.Hour), ok: true}, forDur: 2 * time.Hour, wantFiring: true},
		{name: "within rule limit", src: fakeCPBackups{newest: now.Add(-4 * 24 * time.Hour), ok: true}, forDur: 7 * 24 * time.Hour},
		{name: "no snapshot yet is quiet", src: fakeCPBackups{}},
		{name: "read error", src: fakeCPBackups{err: errors.New("boom")}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			next, notice, err := EvaluateControlPlaneBackupStale(tc.src, Rule{ID: "r1", ForDuration: tc.forDur}, 0, now)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if next.Firing != tc.wantFiring {
				t.Errorf("Firing = %v, want %v", next.Firing, tc.wantFiring)
			}
			if (notice != "") != tc.wantFiring {
				t.Errorf("notice = %q, want non-empty=%v", notice, tc.wantFiring)
			}
		})
	}
}
