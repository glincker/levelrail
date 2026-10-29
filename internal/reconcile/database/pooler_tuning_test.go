package database

import "testing"

func TestPoolerEnv_PerDatabaseOverridesEnvDefault(t *testing.T) {
	t.Setenv(EnvPoolerDefaultPoolSize, "10")
	t.Setenv(EnvPoolerMaxClientConn, "100")
	env := poolerEnv("u", "p", 25, 0)
	if env["DEFAULT_POOL_SIZE"] != "25" {
		t.Fatalf("pool size = %q, want the per-database 25", env["DEFAULT_POOL_SIZE"])
	}
	if env["MAX_CLIENT_CONN"] != "100" {
		t.Fatalf("max client conn = %q, want the env default 100", env["MAX_CLIENT_CONN"])
	}
}
