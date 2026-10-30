package setupguide

import "testing"

func TestDeriveSecurityReview(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want State
	}{
		{"not acknowledged", Input{}, StateTodo},
		{"acknowledged", Input{SecurityReviewed: true}, StateDone},
		{"source unreadable", Input{Unavailable: map[StepID]bool{StepSecurityReview: true}}, StateNotApplicable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stateOf(t, Derive(tt.in), StepSecurityReview).State; got != tt.want {
				t.Fatalf("state = %s, want %s", got, tt.want)
			}
		})
	}
}
