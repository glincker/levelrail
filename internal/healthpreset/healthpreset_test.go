package healthpreset

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

func TestCatalogIsConsistent(t *testing.T) {
	c := Current()
	seen := map[string]bool{}
	for _, p := range c.Presets {
		if seen[p.ID] || p.Path == "" || p.Path[0] != '/' {
			t.Fatalf("bad preset %+v", p)
		}
		seen[p.ID] = true
		if got, ok := Get(p.ID); !ok || got != p {
			t.Fatalf("Get(%q) = %+v, %v", p.ID, got, ok)
		}
	}
	if _, ok := Get("nope"); ok {
		t.Fatal("unknown id resolved")
	}
	if c.Liveness.Failures == 0 || c.Readiness.IntervalSeconds == 0 {
		t.Fatalf("timing not filled: %+v", c)
	}
}

// The create forms still carry their own default timing in TypeScript; this keeps it equal to TimingFor.
func TestWebTimingDefaultsMatch(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "web", "src", "lib", "healthCheckDefaults.ts"))
	if err != nil {
		t.Fatal(err)
	}
	num := func(name string) int {
		m := regexp.MustCompile(name + `\s*=\s*(\d+)(?: \* NS_PER_SECOND)?`).FindSubmatch(raw)
		if m == nil {
			t.Fatalf("%s not found", name)
		}
		n, _ := strconv.Atoi(string(m[1]))
		return n
	}
	r, l := TimingFor("readiness"), TimingFor("liveness")
	if num("READINESS_INTERVAL_NS") != r.IntervalSeconds || num("READINESS_TIMEOUT_NS") != r.TimeoutSeconds ||
		num("LIVENESS_INTERVAL_NS") != l.IntervalSeconds || num("LIVENESS_FAILURES") != l.Failures {
		t.Fatalf("web defaults drifted from healthpreset timing: %+v %+v", r, l)
	}
}
