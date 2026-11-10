package dbparams

import (
	"strings"
	"testing"
)

func TestDatabaseParamsAllowlistAcceptsValidPerEngine(t *testing.T) {
	cases := []struct{ engine, name, in, want string }{
		{"postgres", "shared_buffers", "256mb", "256MB"},
		{"postgres", "max_connections", "200", "200"},
		{"postgres", "jit", "false", "off"},
		{"postgres", "checkpoint_completion_target", "0.70", "0.7"},
		{"mysql", "innodb_buffer_pool_size", "1g", "1G"},
		{"mariadb", "innodb_flush_log_at_trx_commit", "2", "2"},
		{"redis", "maxmemory", "512MB", "512mb"},
		{"valkey", "maxmemory-policy", "allkeys-lru", "allkeys-lru"},
		{"redis", "lazyfree-lazy-eviction", "on", "yes"},
		{"postgres", "max_wal_size", "32MB", "32MB"},
		{"mongodb", "wiredTigerCacheSizeGB", "1.5", "1.5"},
	}
	for _, c := range cases {
		got, err := ValidateOne(c.engine, c.name, c.in, Limits{})
		if err != nil || got != c.want {
			t.Errorf("%s %s=%q: got %q, %v; want %q", c.engine, c.name, c.in, got, err, c.want)
		}
	}
}

func TestDatabaseParamsRejectsInjectionAndBadTypes(t *testing.T) {
	bad := []struct{ engine, name, in string }{
		{"postgres", "work_mem", "4MB\nlisten_addresses=*"},
		{"postgres", "work_mem", "4MB -c ssl=off"},
		{"postgres", "work_mem", "'4MB'"},
		{"postgres", "work_mem", "4MB;"},
		{"postgres", "statement_timeout", "1=1"},
		{"mysql", "max_allowed_packet", "64M\\"},
		{"redis", "maxmemory-policy", "noeviction\nrequirepass x"},
		{"redis", "maxmemory", "\"1gb\""},
		{"mongodb", "slowms", "100 --noauth"},
		{"postgres", "max_connections", "abc"},
		{"postgres", "max_connections", "5"},
		{"postgres", "shared_buffers", "64kB"},
		{"postgres", "shared_buffers", "10PB"},
		{"redis", "appendfsync", "sometimes"},
		{"postgres", "jit", "maybe"},
		{"mongodb", "wiredTigerCacheSizeGB", "NaN"},
		{"postgres", "max_connections", ""},
		{"postgres", "max_wal_size", "16MB"},
	}
	for _, c := range bad {
		if v, err := ValidateOne(c.engine, c.name, c.in, Limits{}); err == nil {
			t.Errorf("%s %s=%q accepted as %q", c.engine, c.name, c.in, v)
		}
	}
}

func TestDatabaseParamsDeniedListMustBeRejected(t *testing.T) {
	must := map[string][]string{
		"postgres": {"listen_addresses", "ssl_cert_file", "archive_command", "shared_preload_libraries", "include", "include_dir", "hba_file", "data_directory", "log_min_duration_statement"},
		"mysql":    {"local_infile", "secure_file_priv", "bind_address", "plugin_load", "init_file", "skip_grant_tables", "sql_mode"},
		"mariadb":  {"local_infile", "secure_file_priv"},
		"redis":    {"requirepass", "rename-command", "dir", "dbfilename", "loadmodule", "bind", "protected-mode", "enable-debug-command", "appendonly"},
		"valkey":   {"requirepass", "rename-command", "appendonly"},
		"mongodb":  {"bind_ip", "noauth", "keyFile", "setParameter", "dbpath"},
	}
	for engine, names := range must {
		for _, n := range names {
			if _, err := ValidateOne(engine, n, "1", Limits{}); err == nil {
				t.Errorf("%s %s accepted", engine, n)
			}
		}
	}
	if _, err := Validate("memcached", map[string]string{"x": "1"}, Limits{}); err == nil {
		t.Error("memcached accepted parameters")
	}
}

func TestDatabaseParamsConnectionCeilingFromLimits(t *testing.T) {
	if _, err := ValidateOne("postgres", "max_connections", "600", Limits{MaxConnections: 500}); err == nil {
		t.Fatal("600 accepted above a 500 ceiling")
	}
	if _, err := ValidateOne("redis", "maxclients", "500", Limits{MaxConnections: 500}); err != nil {
		t.Fatalf("500 at the ceiling refused: %v", err)
	}
}

func TestDatabaseParamsMemoryCeiling(t *testing.T) {
	lim := Limits{MaxMemoryBytes: MemoryCeiling(gib, 4096)}
	if _, err := ValidateOne("postgres", "shared_buffers", "1GB", lim); err == nil {
		t.Error("1GB accepted above 75 percent of a 1GB limit")
	}
	if _, err := ValidateOne("postgres", "shared_buffers", "512MB", lim); err != nil {
		t.Errorf("512MB refused: %v", err)
	}
	if _, err := ValidateOne("mongodb", "wiredTigerCacheSizeGB", "1", lim); err == nil {
		t.Error("1GB cache accepted above the ceiling")
	}
	noLimit := Limits{MaxMemoryBytes: MemoryCeiling(0, 4096)}
	if _, err := ValidateOne("redis", "maxmemory", "8gb", noLimit); err == nil {
		t.Error("8gb accepted above the 4096MB admin ceiling")
	}
	if got := Args("postgres", map[string]string{"work_mem": "1GB"}, lim); len(got) != 0 {
		t.Errorf("render kept an entry above the ceiling: %v", got)
	}
}

func TestDatabaseParamsArgsRenderSeparateTokens(t *testing.T) {
	got := strings.Join(Args("postgres", map[string]string{"work_mem": "8MB", "bogus": "1"}, Limits{}), " ")
	if got != "-c work_mem=8MB" {
		t.Errorf("postgres args = %q", got)
	}
	got = strings.Join(Args("redis", map[string]string{"maxmemory": "1gb", "requirepass": "x"}, Limits{}), "|")
	if got != "--maxmemory|1gb" {
		t.Errorf("redis args = %q", got)
	}
	if Fingerprint(nil) != "" {
		t.Error("empty set has a fingerprint")
	}
}

func TestIncompatibleForVersion(t *testing.T) {
	params := map[string]string{"jit": "off", "max_connections": "200", "work_mem": "8MB"}
	if got := Incompatible("postgres", "10.23", params, Limits{}); len(got) != 1 || got[0] != "jit" {
		t.Fatalf("postgres 10 = %v, want [jit]", got)
	}
	if got := Incompatible("postgres", "16.4", params, Limits{}); len(got) != 0 {
		t.Fatalf("postgres 16 = %v, want none", got)
	}
	if got := Incompatible("postgres", "16.4", params, Limits{MaxConnections: 150}); len(got) != 1 || got[0] != "max_connections" {
		t.Fatalf("capped connections = %v, want [max_connections]", got)
	}
	if got := Incompatible("postgres", "latest", map[string]string{"jit": "on"}, Limits{}); len(got) != 0 {
		t.Fatalf("unknown major = %v, want none", got)
	}
	if got := Incompatible("postgres", "16", map[string]string{"archive_command": "x"}, Limits{}); len(got) != 1 {
		t.Fatalf("denied name = %v, want it listed", got)
	}
}
