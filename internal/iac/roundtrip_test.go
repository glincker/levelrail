package iac

import (
	"context"
	"strings"
	"testing"
)

const roundTripApp = `version: 1
kind: App
metadata: {name: api}
spec:
  service:
    build: {type: image, image: "ghcr.io/acme/api:1"}
    port: 8080
    replicas: 3
    strategy: rolling
%s`

// Each service setting added in v0.8.10 must be saved by apply, printed by
// export and diffed by plan; saved reads the setting back from the fake.
func TestV0810SettingsRoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		yaml   string
		saved  func(f *fakeCP) bool
		export string
		change string
	}{
		{
			name: "rollout",
			yaml: "    rollout: {max_surge: 25%, max_unavailable: \"1\", min_available: \"2\"}\n",
			saved: func(f *fakeCP) bool {
				return f.rollouts["api"]["min_available"] == "2" && f.rollouts["api"]["max_surge"] == "25%"
			},
			export: "min_available: \"2\"",
			change: "    rollout: {max_surge: 25%, max_unavailable: \"1\", min_available: \"1\"}\n",
		},
		{
			name:   "labels",
			yaml:   "    labels: {team: core}\n",
			saved:  func(f *fakeCP) bool { l, _ := f.apps["api"]["labels"].(map[string]any); return l["team"] == "core" },
			export: "team: core",
			change: "    labels: {team: edge}\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake()
			opts := Options{Source: "git"}
			src := strings.Replace(roundTripApp, "%s", tc.yaml, 1)
			applyAll(t, f, src, opts)
			if !tc.saved(f) {
				t.Fatalf("apply did not save %s: %v %v", tc.name, f.apps["api"], f.rollouts)
			}
			if p := planFor(t, f, src, opts); p.Summary.Create+p.Summary.Update+p.Summary.Delete != 0 {
				t.Fatalf("replan not empty: %+v", p.Changes)
			}
			exp, err := Export(context.Background(), f, ExportOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(exp.Join(), tc.export) {
				t.Fatalf("export lacks %q:\n%s", tc.export, exp.Join())
			}
			changed := strings.Replace(roundTripApp, "%s", tc.change, 1)
			if p := planFor(t, f, changed, opts); p.Summary.Update != 1 {
				t.Fatalf("plan missed the %s diff: %+v", tc.name, p.Summary)
			}
		})
	}
}
