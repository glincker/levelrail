package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodYAML = `
version: 1
name: release
on:
  push:
    branches: [main]
  pull_request:
  manual:
    inputs:
      env: { default: staging, options: [staging, prod] }
stages: [test, build, deploy]
templates:
  gotest:
    params: [pkg]
    steps:
      - run: go test ${{ inputs.pkg }}
jobs:
  test:
    stage: test
    image: golang:1.23
    matrix:
      go: ["1.22", "1.23"]
      exclude:
        - go: "1.22"
    steps:
      - uses: template/gotest
        with: { pkg: ./... }
  build:
    stage: build
    needs: test
    steps:
      - uses: build
        id: img
  ship:
    stage: deploy
    needs: [build]
    if: success() && ref == 'refs/heads/main'
    steps:
      - uses: approval
        with: { approvers: deploy }
      - uses: deploy
        with: { service: web }
`

func TestValidateAccepts(t *testing.T) {
	def, issues := Validate([]byte(goodYAML))
	if len(issues) > 0 {
		t.Fatalf("issues: %v", issues)
	}
	if got := def.Jobs["test"].Steps[0].Run; got != "go test ./..." {
		t.Errorf("template not expanded: %q", got)
	}
	if def.On.PullRequest == nil {
		t.Error("bare `pull_request:` must enable the trigger")
	}
	if strings.Join(def.JobOrder, ",") != "test,build,ship" {
		t.Errorf("job order = %v", def.JobOrder)
	}
}

func TestValidateRejects(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"bad yaml", "jobs: [", "invalid yaml"},
		{"wrong version", "version: 2\njobs:\n  a:\n    steps:\n      - run: x\n", "version"},
		{"unknown key", "version: 1\nbogus: 1\njobs:\n  a:\n    steps:\n      - run: x\n", "bogus"},
		{"unknown need", "version: 1\njobs:\n  a:\n    image: x\n    needs: [zz]\n    steps:\n      - run: x\n", "unknown job"},
		{"cycle", "version: 1\njobs:\n  a:\n    image: x\n    needs: [b]\n    steps:\n      - run: x\n  b:\n    image: x\n    needs: [a]\n    steps:\n      - run: x\n", "cycle"},
		{"missing image", "version: 1\njobs:\n  a:\n    steps:\n      - run: x\n", "image is required"},
		{"bad cron", "version: 1\non:\n  schedule: ['nope']\njobs:\n  a:\n    image: x\n    steps:\n      - run: x\n", "on.schedule"},
		{"bad if", "version: 1\njobs:\n  a:\n    image: x\n    if: '(a &&'\n    steps:\n      - run: x\n", "condition"},
		{"unknown kind", "version: 1\njobs:\n  a:\n    steps:\n      - uses: teleport\n", "unknown step kind"},
		{"approval after run", "version: 1\njobs:\n  a:\n    image: x\n    steps:\n      - run: x\n      - uses: approval\n", "approval steps must come before"},
		{"deploy w/o promote args", "version: 1\njobs:\n  a:\n    steps:\n      - uses: promote\n", "requires with.from"},
		{"unknown template", "version: 1\njobs:\n  a:\n    image: x\n    steps:\n      - uses: template/nope\n", "unknown template"},
		{"unknown stage", "version: 1\nstages: [a]\njobs:\n  j:\n    stage: b\n    image: x\n    steps:\n      - run: x\n", "not listed in stages"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			def, issues := Validate([]byte(tc.yaml))
			if def != nil || len(issues) == 0 {
				t.Fatalf("expected issues, got def=%v", def != nil)
			}
			var all []string
			for _, i := range issues {
				all = append(all, i.String())
			}
			if !strings.Contains(strings.Join(all, "\n"), tc.want) {
				t.Errorf("issues %v do not mention %q", all, tc.want)
			}
		})
	}
}

func TestValidateReportsLines(t *testing.T) {
	_, issues := Validate([]byte("version: 1\njobs:\n  a:\n    image: x\n    needs: [zz]\n    steps:\n      - run: x\n"))
	if len(issues) != 1 || issues[0].Line != 5 {
		t.Fatalf("issues = %+v, want one at line 5", issues)
	}
}

func TestEvalCondition(t *testing.T) {
	sc := Scope{Success: true, Vars: map[string]string{"ref": "refs/heads/main", "needs.build.result": "success", "matrix.go": "1.23"}}
	tests := []struct {
		expr string
		want bool
	}{
		{"", true},
		{"success()", true},
		{"failure()", false},
		{"always()", true},
		{"ref == 'refs/heads/main'", true},
		{"ref != 'refs/heads/main'", false},
		{"!failure() && matrix.go == '1.23'", true},
		{"${{ startsWith(ref, 'refs/heads/') }}", true},
		{"contains(ref, 'dev') || needs.build.result == 'success'", true},
		{"(failure() || cancelled()) && always()", false},
	}
	for _, tc := range tests {
		got, err := EvalCondition(tc.expr, sc)
		if err != nil || got != tc.want {
			t.Errorf("%q = %v, %v; want %v", tc.expr, got, err, tc.want)
		}
	}
	if _, err := EvalCondition("nope(1)", sc); err == nil {
		t.Error("unknown function should error")
	}
}

func TestInterpolate(t *testing.T) {
	sc := Scope{Vars: map[string]string{"sha": "abc", "secrets.TOKEN": "s3"}}
	if got, err := Interpolate("v-${{ sha }}-${{secrets.TOKEN}}-${{ missing }}", sc); err != nil || got != "v-abc-s3-" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := Interpolate("${{ secrets.NOPE }}", sc); err == nil {
		t.Error("missing secret should error")
	}
}

func TestMatrixExpand(t *testing.T) {
	m := &Matrix{
		Vars:    map[string]StringList{"os": {"a", "b"}, "go": {"1", "2"}},
		Exclude: []map[string]string{{"os": "b", "go": "2"}},
		Include: []map[string]string{{"os": "z", "go": "9"}},
	}
	combos, err := m.Expand()
	if err != nil || len(combos) != 4 {
		t.Fatalf("combos = %v, %v", combos, err)
	}
	if k := ComboKey("t", Combo{"os": "a", "go": "1"}); k != "t[go=1,os=a]" {
		t.Errorf("key = %s", k)
	}
	if c, _ := (*Matrix)(nil).Expand(); len(c) != 1 {
		t.Errorf("nil matrix should yield one combo")
	}
}

func TestTriggersMatch(t *testing.T) {
	def, issues := Validate([]byte("version: 1\non:\n  push:\n    branches: ['release/*']\n  tag:\n    patterns: ['v*']\njobs:\n  a:\n    steps:\n      - uses: notify\n        with: { message: hi }\n"))
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	tests := []struct {
		ev   Event
		want bool
	}{
		{Event{Kind: TriggerPush, Branch: "release/1"}, true},
		{Event{Kind: TriggerPush, Branch: "main"}, false},
		{Event{Kind: TriggerTag, Tag: "v1.2"}, true},
		{Event{Kind: TriggerTag, Tag: "x1"}, false},
		{Event{Kind: TriggerManual}, false},
	}
	for _, tc := range tests {
		if got := def.On.Matches(tc.ev); got != tc.want {
			t.Errorf("%+v = %v", tc.ev, got)
		}
	}
}

func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".ci", "pipelines"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"b.yaml", "a.yml", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, ".ci", "pipelines", n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := Discover(dir, "brand")
	if err != nil || len(files) != 2 || filepath.Base(files[0]) != "a.yml" {
		t.Fatalf("files = %v, %v", files, err)
	}
	if got := DiscoverDirs("brand"); got[len(got)-1] != filepath.Join(".brand", "pipelines") {
		t.Errorf("dirs = %v", got)
	}
}
