package deploysafety

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// ReasonCrashLoopBackOff is the Ready condition reason while a container that
// keeps exiting waits out its restart delay.
const ReasonCrashLoopBackOff = "CrashLoopBackOff"

var crashLoopMessageRe = regexp.MustCompile(`\((\d+) restarts\), next restart in (\S+)`)

// CrashLoopMessage is the condition message the controller writes; the CLI
// reads it back with ParseCrashLoop, so keep both in this file.
func CrashLoopMessage(container string, restarts int, wait time.Duration) string {
	return fmt.Sprintf("%q keeps exiting (%d restarts), next restart in %s", container, restarts, wait.Round(time.Second))
}

// ParseCrashLoop extracts the restart count and the delay before the next
// restart from a CrashLoopMessage.
func ParseCrashLoop(msg string) (restarts int, wait time.Duration, ok bool) {
	m := crashLoopMessageRe.FindStringSubmatch(msg)
	if m == nil {
		return 0, 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, 0, false
	}
	d, err := time.ParseDuration(m[2])
	if err != nil {
		return 0, 0, false
	}
	return n, d, true
}
