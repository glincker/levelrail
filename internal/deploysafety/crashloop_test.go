package deploysafety

import (
	"testing"
	"time"
)

func TestCrashLoopMessageRoundTrip(t *testing.T) {
	msg := CrashLoopMessage("web-1", 4, 90*time.Second)
	n, wait, ok := ParseCrashLoop(msg)
	if !ok || n != 4 || wait != 90*time.Second {
		t.Fatalf("ParseCrashLoop(%q) = %d, %s, %t", msg, n, wait, ok)
	}
	if _, _, ok := ParseCrashLoop("exited during readiness"); ok {
		t.Fatal("unrelated message parsed as crash loop")
	}
}
