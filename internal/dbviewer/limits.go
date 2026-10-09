package dbviewer

import (
	"os"
	"strconv"
	"time"
)

// Environment variables that tune the console. Each has a default so an
// unset variable behaves the same on every install.
const (
	EnvQueryTimeout  = "APP_DB_QUERY_TIMEOUT"
	EnvQueryMaxRows  = "APP_DB_QUERY_MAX_ROWS"
	EnvQueryMaxBytes = "APP_DB_QUERY_MAX_BYTES"
	EnvQueryMaxCell  = "APP_DB_QUERY_MAX_CELL_BYTES"
	EnvSchemaMaxRows = "APP_DB_SCHEMA_MAX_ROWS"
	EnvQueryMaxSQL   = "APP_DB_QUERY_MAX_SQL_BYTES"
	EnvHistoryKeep   = "APP_DB_QUERY_HISTORY_KEEP"
)

const (
	defaultTimeout   = 15 * time.Second
	defaultMaxRows   = 1000
	defaultMaxBytes  = 4 << 20
	defaultMaxCell   = 64 << 10
	defaultSchemaMax = 5000
	defaultHistory   = 200
	defaultMaxSQL    = 64 << 10
)

// Limits bounds one console execution.
type Limits struct {
	Timeout      time.Duration
	MaxRows      int
	MaxBytes     int
	MaxCellBytes int
	SchemaRows   int
	SQLBytes     int
}

// LimitsFromEnv reads the limits, falling back to defaults for unset or
// invalid values.
func LimitsFromEnv(lookup func(string) (string, bool)) Limits {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	return Limits{
		Timeout:      envDuration(lookup, EnvQueryTimeout, defaultTimeout),
		MaxRows:      envInt(lookup, EnvQueryMaxRows, defaultMaxRows),
		MaxBytes:     envInt(lookup, EnvQueryMaxBytes, defaultMaxBytes),
		MaxCellBytes: envInt(lookup, EnvQueryMaxCell, defaultMaxCell),
		SchemaRows:   envInt(lookup, EnvSchemaMaxRows, defaultSchemaMax),
		SQLBytes:     envInt(lookup, EnvQueryMaxSQL, defaultMaxSQL),
	}
}

// HistoryKeepFromEnv is how many history rows to keep per user and database.
func HistoryKeepFromEnv(lookup func(string) (string, bool)) int {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	return envInt(lookup, EnvHistoryKeep, defaultHistory)
}

func envInt(lookup func(string) (string, bool), key string, def int) int {
	raw, ok := lookup(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envDuration(lookup func(string) (string, bool), key string, def time.Duration) time.Duration {
	raw, ok := lookup(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
