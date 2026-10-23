package selfupgrade

import "testing"

func TestClassifyFailure(t *testing.T) {
	cases := []struct{ step, reason, cause, primary string }{
		{StepDownload, "write: no space left on device", CauseDiskFull, FixActionCleanup},
		{StepDownload, "dial tcp: lookup x: no such host", CauseNetwork, FixActionRetry},
		{StepHealth, "unhealthy after 60s", CauseNotHealthy, FixActionRollback},
		{StepChecksum, "mismatch", CauseChecksum, FixActionRetry},
		{StepSignature, "bad", CauseSignature, FixActionNone},
		{StepMigrationCheck, "newer schema", CauseMigration, FixActionRollback},
		{"weird", "???", CauseUnknown, FixActionRetry},
	}
	for _, c := range cases {
		got := ClassifyFailure(c.step, c.reason)
		if got.Cause != c.cause || got.Primary != c.primary {
			t.Errorf("%s/%q: got %s/%s want %s/%s", c.step, c.reason, got.Cause, got.Primary, c.cause, c.primary)
		}
	}
}
