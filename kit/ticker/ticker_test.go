package ticker

import (
	"testing"
	"time"
)

func TestSafe(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ in, want time.Duration }{
		"zero":     {0, DefaultInterval},
		"negative": {-time.Second, DefaultInterval},
		"positive": {5 * time.Second, 5 * time.Second},
	} {
		if got := Safe(tc.in, DefaultInterval); got != tc.want {
			t.Errorf("%s: Safe(%v) = %v, want %v", name, tc.in, got, tc.want)
		}
	}
}
