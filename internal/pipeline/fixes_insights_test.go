package pipeline

import (
	"strings"
	"testing"
)

func TestSuggestFixes(t *testing.T) {
	tests := []struct {
		name, out, wantCode string
		wantKinds           []string
	}{
		{"network", "step 2\nnpm ERR! code ETIMEDOUT\n", "transient_network", []string{FixRerun, FixAddRetries}},
		{"oom", "building\nKilled\nexit code 137", "out_of_memory", []string{FixRaiseMemory}},
		{"disk", "write /x: no space left on device", "disk_full", []string{FixFreeDisk}},
		{"python", "ModuleNotFoundError: No module named 'flask'", "python_module_missing", []string{FixEditPipeline}},
		{"secret", "denied: requested access to the resource is denied", "missing_secret", []string{FixSetSecret}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SuggestFixes("test", tc.out)
			var kinds []string
			for _, f := range got {
				if f.Code == tc.wantCode {
					kinds = append(kinds, f.Kind)
				}
			}
			if strings.Join(kinds, ",") != strings.Join(tc.wantKinds, ",") {
				t.Fatalf("kinds = %v, want %v (all: %+v)", kinds, tc.wantKinds, got)
			}
		})
	}
	if got := SuggestFixes("test", "all good"); len(got) != 0 {
		t.Fatalf("unexpected fixes: %+v", got)
	}
}

func TestApplyFix(t *testing.T) {
	src := "version: 1\njobs:\n  test:\n    image: node:22 # keep\n    steps:\n      - run: npm test\n"
	for _, kind := range []string{FixAddRetries, FixRaiseTimeout, FixRaiseMemory} {
		out, err := ApplyFix([]byte(src), "test", kind)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if _, issues := Validate(out); len(issues) > 0 {
			t.Fatalf("%s produced invalid file: %v\n%s", kind, issues, out)
		}
		if !strings.Contains(string(out), "# keep") {
			t.Fatalf("%s dropped a comment:\n%s", kind, out)
		}
	}
	if _, err := ApplyFix([]byte(src), "nope", FixAddRetries); err == nil {
		t.Fatal("expected unknown job error")
	}
	if _, err := ApplyFix([]byte(src), "test", FixSetSecret); err == nil {
		t.Fatal("expected non-auto kind error")
	}
}

func TestComputeInsights(t *testing.T) {
	ok := func(k string, d int64) JobSample {
		return JobSample{Key: k, Status: sampleOK, DurationSeconds: d, Attempt: 1}
	}
	bad := func(k string, d int64) JobSample {
		return JobSample{Key: k, Status: sampleFailed, DurationSeconds: d, Attempt: 1}
	}
	runs := []RunSample{
		{RunID: "r3", CommitSHA: "c2", Jobs: []JobSample{ok("build", 90), ok("test[node=22]", 70)}},
		{RunID: "r2", CommitSHA: "c1", Jobs: []JobSample{ok("build", 60), bad("test[node=22]", 20)}},
		{RunID: "r1", CommitSHA: "c1", Jobs: []JobSample{ok("build", 60), ok("test[node=22]", 50)}},
	}
	got := ComputeInsights(runs)
	byJob := map[string]JobInsight{}
	for _, j := range got.Jobs {
		byJob[j.Job] = j
	}
	if !byJob["test"].Flaky || byJob["build"].Flaky {
		t.Fatalf("flaky flags wrong: %+v", got.Jobs)
	}
	if got.DiffBaseID != "r1" {
		t.Fatalf("diff base = %q, want r1 (r2 had a failure)", got.DiffBaseID)
	}
	for _, d := range got.Diff {
		if d.Job == "build" && (d.DeltaSeconds != 30 || d.DeltaPercent != 50) {
			t.Fatalf("build delta wrong: %+v", d)
		}
	}
}
