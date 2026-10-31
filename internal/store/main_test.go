package store

import (
	"os"
	"testing"
)

// Fixtures are throwaway files, so skipping fsync keeps migration-heavy tests off the disk.
func TestMain(m *testing.M) {
	_ = os.Setenv(EnvDBSynchronous, "OFF")
	os.Exit(m.Run())
}
