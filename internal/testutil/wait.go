// Package testutil holds helpers shared by test code across packages.
package testutil

import (
	"testing"
	"time"
)

// WaitFor polls cond every interval until it holds, failing the test
// once timeout passes so slow CI shows what was awaited.
func WaitFor(t testing.TB, timeout, interval time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting for %s", timeout, what)
		}
		time.Sleep(interval)
	}
}
