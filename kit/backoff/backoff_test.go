package backoff

import (
	"testing"
	"time"
)

func TestExponential(t *testing.T) {
	s := time.Second
	cases := []struct {
		name      string
		base, max time.Duration
		attempt   int
		want      time.Duration
	}{
		{"first", s, 30 * s, 1, s},
		{"zero attempt", s, 30 * s, 0, s},
		{"doubles", s, 30 * s, 4, 8 * s},
		{"capped", s, 30 * s, 10, 30 * s},
		{"base above max", 60 * s, 30 * s, 1, 30 * s},
		{"zero max", s, 0, 3, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Exponential(c.base, c.max, c.attempt); got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}
