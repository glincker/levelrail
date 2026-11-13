package iac

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
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
			name:   "files",
			yaml:   "    files:\n      - {path: /etc/app.conf, content: \"a=1\"}\n",
			saved:  func(f *fakeCP) bool { return f.files["api"]["/etc/app.conf"]["content"] == "a=1" },
			export: "path: /etc/app.conf",
			change: "    files:\n      - {path: /etc/app.conf, content: \"a=2\"}\n",
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

// Every stable kind must survive export, apply and export again unchanged.
func TestRoundTripStableKinds(t *testing.T) {
	cases := []struct {
		kind   Kind
		doc    string
		setup  func(f *fakeCP)
		export ExportOptions
		// want overrides the kind marker for kinds the export folds into another document.
		want string
	}{
		{kind: KindProject, doc: "version: 1\nkind: Project\nmetadata: {name: shop}\nspec: {env: {REGION: eu}}\n", export: ExportOptions{IncludeEnvValues: true}},
		{kind: KindEnvironment, doc: "version: 1\nkind: Project\nmetadata: {name: shop}\n---\nversion: 1\nkind: Environment\nmetadata: {name: production}\nspec: {project: shop, protected: true, env: {LOG_LEVEL: info}}\n", export: ExportOptions{IncludeEnvValues: true}},
		{kind: KindTag, doc: "version: 1\nkind: Tag\nmetadata: {name: frontend}\n---\nversion: 1\nkind: App\nmetadata: {name: api}\nspec:\n  tags: [frontend]\n  service:\n    build: {type: image, image: x}\n    port: 80\n", want: "tags:"},
		{kind: KindDatabase, doc: "version: 1\nkind: Database\nmetadata: {name: main-db}\nspec: {engine: postgres, version: \"16\"}\n"},
		{kind: KindApp, doc: strings.Replace(roundTripApp, "%s", "    env: {LOG_LEVEL: info}\n", 1), export: ExportOptions{IncludeEnvValues: true}},
		{kind: KindDomain, doc: strings.Replace(roundTripApp, "%s", "", 1) + "---\nversion: 1\nkind: Domain\nmetadata: {name: www.example.com}\nspec: {app: api}\n", want: "- www.example.com"},
		{kind: KindNode, doc: nodeDoc, setup: func(f *fakeCP) { addNodes(f, "worker-1") }, export: ExportOptions{Node: "worker-1"}},
		{kind: KindSetting, doc: settingDoc(runtimesettings.KeyHeartbeatJobAttempts, "value: 7"), setup: addSettingsWithList, export: ExportOptions{Settings: true}},
	}
	covered := map[Kind]bool{}
	for _, tc := range cases {
		covered[tc.kind] = true
		t.Run(string(tc.kind), func(t *testing.T) {
			f := newFake()
			if tc.setup != nil {
				tc.setup(f)
			}
			opts := Options{Source: "git"}
			applyAll(t, f, tc.doc, opts)
			first, err := Export(context.Background(), f, tc.export)
			if err != nil {
				t.Fatal(err)
			}
			joined := first.Join()
			want := tc.want
			if want == "" {
				want = "kind: " + string(tc.kind)
			}
			if !strings.Contains(joined, want) {
				t.Fatalf("export lacks %q:\n%s", want, joined)
			}
			f.writes = nil
			if out := applyAll(t, f, joined, opts); out.Applied != 0 || f.writeCount() != 0 {
				t.Fatalf("applying the export changed things: %v %+v", f.writes, out.Plan.Changes)
			}
			second, err := Export(context.Background(), f, tc.export)
			if err != nil {
				t.Fatal(err)
			}
			if second.Join() != joined {
				t.Fatalf("export not stable:\n%s\n---\n%s", joined, second.Join())
			}
		})
	}
	for _, k := range Kinds {
		if KindStability(k) == StabilityStable && !covered[k] {
			t.Errorf("stable kind %s has no round trip case", k)
		}
	}
}

// addSettingsWithList adds the list route the settings export reads.
func addSettingsWithList(f *fakeCP) {
	live := addSettings(f)
	f.route("GET", `/api/v1/settings/runtime`, func(_ []string, _ map[string]any) (any, error) {
		rows := make([]*runtimesettings.Setting, 0, len(live))
		for _, s := range live {
			rows = append(rows, s)
		}
		return map[string]any{"settings": rows}, nil
	})
}
