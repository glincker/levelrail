package dbprovision

import (
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		tail string
		exit int
		oom  bool
		want string
	}{
		{"setpriv capability", "setpriv: apply bounding set: Operation not permitted", 1, false, CausePermission},
		{"data dir permission", "initdb: error: could not change permissions: Permission denied", 1, false, CausePermission},
		{"exit 137", "", 137, false, CauseOutOfMemory},
		{"oom flag", "", 1, true, CauseOutOfMemory},
		{"kernel oom text", "Out of memory: Killed process 12", 1, false, CauseOutOfMemory},
		{"port", "Could not create server TCP listening socket *:6379: bind: Address already in use", 1, false, CausePortInUse},
		{"pg incompatible", "FATAL: database files are incompatible with server", 1, false, CauseBadVolume},
		{"redis aof", "Bad file format reading the append only file", 1, false, CauseBadVolume},
		{"bad config", "FATAL: unrecognized configuration parameter \"foo\"", 1, false, CauseBadConfig},
		{"unknown", "something odd happened", 1, false, ""},
		{"empty clean exit", "", 0, false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.tail, tc.exit, tc.oom)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("want nil, got %q", got.Kind)
				}
				return
			}
			if got == nil || got.Kind != tc.want || got.Fix == nil || got.Hint == "" {
				t.Fatalf("want %q with fix and hint, got %+v", tc.want, got)
			}
		})
	}
}

func TestRedactTail(t *testing.T) {
	in := "start\npassword=hunter2\nurl postgres://app:s3cret@db:5432/x\nAuthorization: Bearer abc.def\nAPI_KEY: 'k-123'\nok line"
	out := RedactTail(in)
	for _, leak := range []string{"hunter2", "s3cret", "abc.def", "k-123"} {
		if strings.Contains(out, leak) {
			t.Fatalf("leaked %q in %q", leak, out)
		}
	}
	if !strings.Contains(out, "ok line") {
		t.Fatalf("dropped plain line: %q", out)
	}
}

func TestRedactTailKeepsLastLines(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 100; i++ {
		b.WriteString("line\n")
	}
	if n := len(strings.Split(RedactTail(b.String()), "\n")); n != maxTailLines {
		t.Fatalf("want %d lines, got %d", maxTailLines, n)
	}
}

func TestViewAttachesCrash(t *testing.T) {
	at := time.Now()
	events := []store.DatabaseProvisionEvent{
		{Step: StepWaitReady, Status: StatusFailed, Detail: "not running", At: at},
		{Step: StepLogTail, Status: StatusInfo, Detail: EncodeCrash(CrashInfo{Tail: "setpriv: Operation not permitted", ExitCode: 1}), At: at},
	}
	a := View("a1", events)
	if a.Failure == nil || a.Failure.LogTail == "" || a.Failure.Cause == nil || a.Failure.Cause.Kind != CausePermission {
		t.Fatalf("unexpected failure: %+v", a.Failure)
	}
}
