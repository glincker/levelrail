package iac

import (
	"context"
	"strings"
	"testing"
)

const placementDoc = `    tolerations:
      - {key: gpu, operator: Equal, value: "true", effect: NoSchedule}
    affinity:
      node_required:
        - {key: zone, op: In, values: [a, b]}
`

func quiet(p Plan) int { return p.Summary.Create + p.Summary.Update + p.Summary.Delete }

func TestPlacementRoundTrip(t *testing.T) {
	f := newFake()
	opts := Options{Source: "git"}
	src := strings.Replace(roundTripApp, "%s", placementDoc, 1)
	applyAll(t, f, src, opts)
	if _, ok := f.apps["api"]["tolerations"]; !ok {
		t.Fatalf("apply did not save tolerations: %v", f.apps["api"])
	}
	if p := planFor(t, f, src, opts); quiet(p) != 0 {
		t.Fatalf("replan not empty: %+v", p.Changes)
	}
	exp, err := Export(context.Background(), f, ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tolerations:", "key: gpu", "node_required:", "values:"} {
		if !strings.Contains(exp.Join(), want) {
			t.Fatalf("export lacks %q:\n%s", want, exp.Join())
		}
	}
	if p := planFor(t, f, exp.Join(), opts); quiet(p) != 0 {
		t.Fatalf("export does not replan clean: %+v", p.Changes)
	}
}

func TestPlacementChangeAndClear(t *testing.T) {
	f := newFake()
	opts := Options{Source: "git"}
	applyAll(t, f, strings.Replace(roundTripApp, "%s", placementDoc, 1), opts)

	moved := strings.Replace(placementDoc, "gpu", "fast", 1)
	if p := planFor(t, f, strings.Replace(roundTripApp, "%s", moved, 1), opts); p.Summary.Update != 1 {
		t.Fatalf("plan missed the toleration change: %+v", p.Summary)
	}

	cleared := strings.Replace(roundTripApp, "%s", "    tolerations: []\n    affinity: {}\n", 1)
	if p := planFor(t, f, cleared, opts); p.Summary.Update != 1 {
		t.Fatalf("plan missed the clear: %+v", p.Summary)
	}
	applyAll(t, f, cleared, opts)
	if p := planFor(t, f, cleared, opts); quiet(p) != 0 {
		t.Fatalf("replan after clear not empty: %+v", p.Changes)
	}

	applyAll(t, f, strings.Replace(roundTripApp, "%s", placementDoc, 1), opts)
	bare := strings.Replace(roundTripApp, "%s", "", 1)
	if p := planFor(t, f, bare, opts); p.Summary.Update != 0 {
		t.Fatalf("undeclared placement must stay unmanaged: %+v", p.Changes)
	}
}
