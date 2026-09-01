// Package dockertest holds test helpers shared by every package whose
// _live_test.go files need a real Docker daemon (or other real,
// network-dependent infra) to run.
package dockertest

import "testing"

// SkipIfShort skips t in short mode; the full, no-short run lives in
// nightly.yml.
func SkipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("real Docker test, skipped in short mode; see nightly.yml for the full run")
	}
}
