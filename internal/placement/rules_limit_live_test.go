package placement

import "testing"

func TestPlacementAffinityTermsLive(t *testing.T) {
	SetAffinityTermsSource(func() int { return 3 })
	t.Cleanup(func() { affinityLive.Store(nil) })
	if got := MaxAffinityTerms(); got != 3 {
		t.Errorf("MaxAffinityTerms = %d, want 3", got)
	}
}
