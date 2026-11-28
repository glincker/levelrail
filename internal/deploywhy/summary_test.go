package deploywhy

import (
	"strings"
	"testing"
)

func TestSummarizeFailureFixtures(t *testing.T) {
	one, oom := 1, 137
	cases := []struct {
		name  string
		in    Input
		wants []string
	}{
		{
			name: "oom",
			in: Input{DeployID: "d1", Failed: true, ExitCode: &oom, OOMKilled: true, BaselineID: "g1",
				Cause:     &Cause{Code: "oom_killed", Text: "the container ran out of memory", Fix: "raise the memory limit"},
				TailLines: []string{"loading index", "Killed", "Killed", "Killed"},
				Changes:   Changes{Image: &Change{From: "web:1", To: "web:2"}, EnvChanged: []string{"CACHE_SIZE"}}},
			wants: []string{"oom_killed", "raise the memory limit", "exit code 137, oom killed", "Killed (x3)", "image web:1 to web:2", "env changed CACHE_SIZE"},
		},
		{
			name: "port in use",
			in: Input{DeployID: "d2", Failed: true, ExitCode: &one, BaselineID: "g1",
				Cause:     &Cause{Code: "port_not_listening", Text: "the port is already in use", Fix: "pick a free port"},
				TailLines: []string{"listen tcp :8080: bind: address already in use"},
				Changes:   Changes{Port: &Change{From: "3000", To: "8080"}}},
			wants: []string{"port_not_listening", "pick a free port", "exit code 1", "address already in use", "port 3000 to 8080"},
		},
		{
			name: "failed healthcheck",
			in: Input{DeployID: "d3", Failed: true, BaselineID: "g1",
				Health:    &Health{Target: "http://web:3000/health", Success: false, StatusCode: 503},
				Cause:     &Cause{Code: "health_check_failed", Text: "the health check never passed", Fix: "check the health path"},
				TailLines: []string{"starting", "ready", "starting"},
				Changes:   Changes{EnvAdded: []string{"B_KEY", "A_KEY"}}},
			wants: []string{"health_check_failed", "check the health path", "ok=false status=503", "starting | ready | starting", "env added A_KEY,B_KEY"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Compute(tc.in)
			if len(r.Summary) != 5 {
				t.Fatalf("summary has %d lines", len(r.Summary))
			}
			all := strings.Join(r.Summary, "\n")
			for _, w := range tc.wants {
				if !strings.Contains(all, w) {
					t.Errorf("summary missing %q:\n%s", w, all)
				}
			}
		})
	}
}

func TestCollapseTailKeepsLastDistinct(t *testing.T) {
	got := CollapseTail([]string{"a", "b", "b", "", "c"}, 2)
	if len(got) != 2 || got[0].Text != "b" || got[0].Count != 2 || got[1].Text != "c" {
		t.Fatalf("unexpected tail: %+v", got)
	}
}
