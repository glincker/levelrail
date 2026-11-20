package diagnose

import (
	"fmt"
	"strings"
	"testing"
)

func TestGenericCrashQuotesLastLogLinesAndExitCode(t *testing.T) {
	var raw []string
	for i := 0; i < 30; i++ {
		raw = append(raw, fmt.Sprintf("odd line %d", i))
	}
	causes := Analyze(Input{Facts: Facts{HasExit: true, ExitCode: 3}, RecentLogLines: raw})
	if len(causes) != 1 || causes[0].Code != CauseCrashloopGeneric {
		t.Fatalf("want one generic crash cause, got %+v", causes)
	}
	ev := causes[0].Evidence
	if len(ev) != 1+crashTailLines {
		t.Fatalf("want exit code plus %d log lines, got %d", crashTailLines, len(ev))
	}
	if ev[0].Excerpt != "exit code 3" || ev[1].Excerpt != "odd line 10" || ev[len(ev)-1].Excerpt != "odd line 29" {
		t.Errorf("unexpected evidence: %+v", ev)
	}
}

func TestOOMExplanationPointsToResourceAdvisor(t *testing.T) {
	c := Analyze(Input{Facts: Facts{OOMKilled: true, HasExit: true, ExitCode: 137, MemoryBytes: 256 * mib}})
	if len(c) == 0 || !strings.Contains(c[0].Explanation, "/resource-advisor") {
		t.Fatalf("OOM cause should link the resource advisor: %+v", c)
	}
}
