package fixit

import (
	"strings"
	"testing"
)

// Built from code points so this file itself passes the dash sweep.
var bannedDashes = string([]rune{0x2013, 0x2014})

func TestExplainEntriesAreWellFormed(t *testing.T) {
	t.Parallel()
	for _, id := range ExplainIDs() {
		e, ok := Explain(id)
		if !ok || e.ID != id {
			t.Fatalf("Explain(%q) = %+v, %v", id, e, ok)
		}
		for name, v := range map[string]string{"plain": e.Plain, "why": e.Why, "next": e.Next} {
			if v == "" || strings.ContainsAny(v, bannedDashes+"!") || !strings.HasSuffix(v, ".") {
				t.Errorf("%s %s = %q: empty, has a banned character or no final period", id, name, v)
			}
		}
		if e.FixID != "" && e.Kind == KindManual {
			t.Errorf("%s has a fix id but kind manual", id)
		}
		if e.FixID == "" && e.Kind == KindAuto {
			t.Errorf("%s is auto without a fix id", id)
		}
		if strings.HasPrefix(e.Next, "Run ") {
			t.Errorf("%s: next must name a button or guided step before a command", id)
		}
	}
	if _, ok := Explain("no_such_check"); ok {
		t.Error("unknown id must not resolve")
	}
}
