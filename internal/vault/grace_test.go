package vault

import (
	"errors"
	"testing"
	"time"
)

func TestResolveWithGrace(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	errRejected := errors.New("permission denied")

	const current, previous = "cred-new", "cred-old"

	tests := []struct {
		name      string
		expires   time.Time
		previous  string
		accepted  map[string]bool
		want      string
		wantErr   bool
		wantCalls []string
	}{
		{
			name:      "new credential accepted without fallback",
			expires:   future,
			previous:  previous,
			accepted:  map[string]bool{current: true, previous: true},
			want:      current,
			wantCalls: []string{current},
		},
		{
			name:      "old credential accepted inside grace",
			expires:   future,
			previous:  previous,
			accepted:  map[string]bool{previous: true},
			want:      previous,
			wantCalls: []string{current, previous},
		},
		{
			name:      "old credential rejected after grace",
			expires:   past,
			previous:  previous,
			accepted:  map[string]bool{previous: true},
			wantErr:   true,
			wantCalls: []string{current},
		},
		{
			name:      "grace 0 means no fallback",
			expires:   time.Time{},
			previous:  "",
			accepted:  map[string]bool{previous: true},
			wantErr:   true,
			wantCalls: []string{current},
		},
		{
			name:      "previous rejected inside grace fails",
			expires:   future,
			previous:  previous,
			accepted:  map[string]bool{},
			wantErr:   true,
			wantCalls: []string{current, previous},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			attempt := func(cred string) (string, error) {
				calls = append(calls, cred)
				if tt.accepted[cred] {
					return "value-from-" + cred, nil
				}
				return "", errRejected
			}

			got, err := ResolveWithGrace(now, tt.expires, current, tt.previous, attempt)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != "value-from-"+tt.want {
				t.Errorf("value = %q, want value from %q", got, tt.want)
			}
			if len(calls) != len(tt.wantCalls) {
				t.Fatalf("attempts = %v, want %v", calls, tt.wantCalls)
			}
			for i := range calls {
				if calls[i] != tt.wantCalls[i] {
					t.Errorf("attempt %d = %q, want %q", i, calls[i], tt.wantCalls[i])
				}
			}
		})
	}
}
