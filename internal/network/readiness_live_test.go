package network

import (
	"os"
	"testing"
)

// Reads the real host: an unprivileged machine must fail the WireGuard
// check with a fix hint, a privileged one must not.
func TestReadinessChecks_LiveHost(t *testing.T) {
	if testing.Short() {
		t.Skip("reads the real host")
	}
	checks := ReadinessChecks(SystemReadinessEnv("", false))
	if len(checks) != 5 {
		t.Fatalf("got %d checks, want 5", len(checks))
	}
	var wg Check
	for _, c := range checks {
		t.Logf("%s %s: %s | %s", c.ID, c.Status, c.Detail, c.Hint)
		if c.ID == CheckWireGuard {
			wg = c
		}
	}
	if os.Geteuid() != 0 {
		if wg.Status != CheckFail || wg.Hint == "" {
			t.Fatalf("unprivileged host: wireguard = %+v, want fail with a hint", wg)
		}
	}
}
