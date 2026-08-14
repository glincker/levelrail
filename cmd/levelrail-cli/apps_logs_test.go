package main

import (
	"strings"
	"testing"
	"time"
)

func TestResolveLogWindow(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		flags    logsWindowFlags
		wantErr  string // substring; empty means no error
		wantFrom time.Time
		wantTo   time.Time
	}{
		{
			name:     "nothing given defaults to the last hour",
			flags:    logsWindowFlags{},
			wantFrom: now.Add(-time.Hour),
			wantTo:   now,
		},
		{
			name:     "since applied against now",
			flags:    logsWindowFlags{since: "30m"},
			wantFrom: now.Add(-30 * time.Minute),
			wantTo:   now,
		},
		{
			name:     "from overrides the default window",
			flags:    logsWindowFlags{from: "2026-08-14T10:00:00Z"},
			wantFrom: time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC),
			wantTo:   now,
		},
		{
			name:     "to overrides now",
			flags:    logsWindowFlags{to: "2026-08-14T11:00:00Z"},
			wantFrom: time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC),
			wantTo:   time.Date(2026, 8, 14, 11, 0, 0, 0, time.UTC),
		},
		{
			name:    "since and from are mutually exclusive",
			flags:   logsWindowFlags{since: "1h", from: "2026-08-14T10:00:00Z"},
			wantErr: "mutually exclusive",
		},
		{
			name:    "invalid since",
			flags:   logsWindowFlags{since: "not-a-duration"},
			wantErr: "--since must be a valid duration",
		},
		{
			name:    "invalid from",
			flags:   logsWindowFlags{from: "not-a-timestamp"},
			wantErr: "--from must be RFC3339",
		},
		{
			name:    "invalid to",
			flags:   logsWindowFlags{to: "not-a-timestamp"},
			wantErr: "--to must be RFC3339",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from, to, err := resolveLogWindow(tt.flags, now)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("resolveLogWindow() error = nil, want substring %q", tt.wantErr)
				}
				if got := err.Error(); !strings.Contains(got, tt.wantErr) {
					t.Errorf("error = %q, want substring %q", got, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveLogWindow() error = %v", err)
			}
			if !from.Equal(tt.wantFrom) {
				t.Errorf("from = %v, want %v", from, tt.wantFrom)
			}
			if !to.Equal(tt.wantTo) {
				t.Errorf("to = %v, want %v", to, tt.wantTo)
			}
		})
	}
}

func TestTailEntries(t *testing.T) {
	entries := []logEntryResource{
		{Message: "one"}, {Message: "two"}, {Message: "three"},
	}

	tests := []struct {
		name string
		tail int
		want []string
	}{
		{name: "zero means no trim", tail: 0, want: []string{"one", "two", "three"}},
		{name: "tail smaller than length", tail: 2, want: []string{"two", "three"}},
		{name: "tail larger than length is a no-op", tail: 10, want: []string{"one", "two", "three"}},
		{name: "tail equal to length is a no-op", tail: 3, want: []string{"one", "two", "three"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tailEntries(entries, tt.tail)
			if len(got) != len(tt.want) {
				t.Fatalf("tailEntries() len = %d, want %d", len(got), len(tt.want))
			}
			for i, e := range got {
				if e.Message != tt.want[i] {
					t.Errorf("entry[%d] = %q, want %q", i, e.Message, tt.want[i])
				}
			}
		})
	}
}
