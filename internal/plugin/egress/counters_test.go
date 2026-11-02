package egress

import "testing"

func TestCountersTallyPerPluginAndReason(t *testing.T) {
	c := NewCounters()
	c.Record("b", ReasonHostNotListed)
	c.Record("a", ReasonBlockedAddr)
	c.Record("a", ReasonBlockedAddr)
	got := c.Snapshot()
	if len(got) != 2 || got[0].Plugin != "a" || got[0].Count != 2 || got[1].Plugin != "b" || got[1].Count != 1 {
		t.Fatalf("snapshot = %+v", got)
	}
}
