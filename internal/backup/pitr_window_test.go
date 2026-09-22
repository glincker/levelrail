package backup

import (
	"errors"
	"testing"
	"time"
)

func TestValidatePITRTarget(t *testing.T) {
	start := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 14, 6, 0, 0, 0, time.UTC)
	win := PITRWindow{HasBaseBackup: true, Start: start, End: end}

	tests := []struct {
		name    string
		win     PITRWindow
		target  time.Time
		wantErr bool
		wantIs  error
	}{
		{
			name:   "inside window",
			win:    win,
			target: start.Add(3 * time.Hour),
		},
		{
			name:   "exactly at start",
			win:    win,
			target: start,
		},
		{
			name:   "exactly at end",
			win:    win,
			target: end,
		},
		{
			name:    "before start",
			win:     win,
			target:  start.Add(-time.Minute),
			wantErr: true,
		},
		{
			name:    "after end",
			win:     win,
			target:  end.Add(time.Minute),
			wantErr: true,
		},
		{
			name:    "no base backup at all",
			win:     PITRWindow{HasBaseBackup: false},
			target:  start,
			wantErr: true,
			wantIs:  ErrNoBaseBackup,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePITRTarget(tt.win, tt.target)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePITRTarget() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Fatalf("ValidatePITRTarget() error = %v, want errors.Is(%v)", err, tt.wantIs)
			}
		})
	}
}
