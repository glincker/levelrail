// Package dbparams is the typed allowlist of engine tuning parameters a
// managed database accepts. Deny by default: a name absent from the
// allowlist is refused, and every accepted value is re-rendered from its
// parsed number or enum, so no caller string ever reaches a command line.
package dbparams

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Kind is how a parameter value is parsed and re-rendered.
type Kind string

// Parameter kinds.
const (
	KindInt   Kind = "int"
	KindFloat Kind = "float"
	KindSize  Kind = "size"
	KindEnum  Kind = "enum"
	KindBool  Kind = "bool"
)

// Family groups engines that share a flag syntax and a parameter list.
type Family string

// Engine families with an allowlist.
const (
	FamilyPostgres Family = "postgres"
	FamilyMySQL    Family = "mysql"
	FamilyRedis    Family = "redis"
	FamilyMongo    Family = "mongodb"
)

// Spec describes one allowlisted parameter. Min and Max are bytes for
// KindSize. ConnectionCap marks the parameter the admin ceiling bounds.
type Spec struct {
	Name          string   `json:"name"`
	Kind          Kind     `json:"kind"`
	Default       string   `json:"default"`
	Min           float64  `json:"min,omitempty"`
	Max           float64  `json:"max,omitempty"`
	Options       []string `json:"options,omitempty"`
	ConnectionCap bool     `json:"connection_cap,omitempty"`
	MemoryCap     bool     `json:"memory_cap,omitempty"`
	Description   string   `json:"description"`
}

// Limits are admin bounds read from live runtime settings per request.
type Limits struct {
	MaxConnections int
	MaxMemoryBytes int64
}

// MemoryCeiling is 75 percent of a memory limit, or the admin ceiling when the database has none.
func MemoryCeiling(limitBytes int64, adminMB int) int64 {
	if limitBytes > 0 {
		return limitBytes / 4 * 3
	}
	return int64(adminMB) * mib
}

const (
	kib = 1024
	mib = 1024 * kib
	gib = 1024 * mib
)

var postgresSpecs = []Spec{
	{Name: "max_connections", Kind: KindInt, Default: "100", Min: 10, Max: 100000, ConnectionCap: true, Description: "Concurrent client connections."},
	{Name: "shared_buffers", Kind: KindSize, Default: "128MB", Min: 128 * kib, Max: 256 * gib, MemoryCap: true, Description: "Memory for the shared buffer cache."},
	{Name: "effective_cache_size", Kind: KindSize, Default: "4GB", Min: 8 * kib, Max: 1024 * gib, MemoryCap: true, Description: "Planner estimate of the OS file cache."},
	{Name: "work_mem", Kind: KindSize, Default: "4MB", Min: 64 * kib, Max: 2 * gib, MemoryCap: true, Description: "Memory per sort or hash operation."},
	{Name: "maintenance_work_mem", Kind: KindSize, Default: "64MB", Min: mib, Max: 16 * gib, MemoryCap: true, Description: "Memory for VACUUM and index builds."},
	{Name: "max_wal_size", Kind: KindSize, Default: "1GB", Min: 32 * mib, Max: 1024 * gib, Description: "WAL size that triggers a checkpoint."},
	{Name: "checkpoint_completion_target", Kind: KindFloat, Default: "0.9", Min: 0, Max: 1, Description: "Fraction of the checkpoint interval to spread writes over."},
	{Name: "random_page_cost", Kind: KindFloat, Default: "4", Min: 0, Max: 1000, Description: "Planner cost of a random page read."},
	{Name: "effective_io_concurrency", Kind: KindInt, Default: "1", Min: 0, Max: 1000, Description: "Concurrent disk reads the planner assumes."},
	{Name: "default_statistics_target", Kind: KindInt, Default: "100", Min: 1, Max: 10000, Description: "Default statistics sample size for ANALYZE."},
	{Name: "statement_timeout", Kind: KindInt, Default: "0", Min: 0, Max: 86400000, Description: "Milliseconds before a statement is cancelled, 0 is off."},
	{Name: "idle_in_transaction_session_timeout", Kind: KindInt, Default: "0", Min: 0, Max: 86400000, Description: "Milliseconds an idle transaction may stay open, 0 is off."},
	{Name: "jit", Kind: KindBool, Default: "on", Options: []string{"on", "off"}, Description: "Just in time compilation of queries."},
}

var mysqlSpecs = []Spec{
	{Name: "max_connections", Kind: KindInt, Default: "151", Min: 10, Max: 100000, ConnectionCap: true, Description: "Concurrent client connections."},
	{Name: "innodb_buffer_pool_size", Kind: KindSize, Default: "128M", Min: 5 * mib, Max: 1024 * gib, MemoryCap: true, Description: "Memory for the InnoDB buffer pool."},
	{Name: "max_allowed_packet", Kind: KindSize, Default: "64M", Min: kib, Max: gib, Description: "Largest packet or row the server accepts."},
	{Name: "tmp_table_size", Kind: KindSize, Default: "16M", Min: kib, Max: 64 * gib, MemoryCap: true, Description: "In memory temporary table limit."},
	{Name: "max_heap_table_size", Kind: KindSize, Default: "16M", Min: 16 * kib, Max: 64 * gib, MemoryCap: true, Description: "MEMORY table size limit."},
	{Name: "sort_buffer_size", Kind: KindSize, Default: "256K", Min: 32 * kib, Max: 4 * gib, MemoryCap: true, Description: "Buffer per sort."},
	{Name: "table_open_cache", Kind: KindInt, Default: "4000", Min: 1, Max: 524288, Description: "Open tables kept for all threads."},
	{Name: "wait_timeout", Kind: KindInt, Default: "28800", Min: 1, Max: 31536000, Description: "Seconds an idle connection stays open."},
	{Name: "interactive_timeout", Kind: KindInt, Default: "28800", Min: 1, Max: 31536000, Description: "Seconds an idle interactive connection stays open."},
	{Name: "innodb_flush_log_at_trx_commit", Kind: KindEnum, Default: "1", Options: []string{"0", "1", "2"}, Description: "Durability of the redo log flush per commit."},
	{Name: "character_set_server", Kind: KindEnum, Default: "utf8mb4", Options: []string{"utf8mb4", "utf8mb3", "latin1"}, Description: "Default server character set."},
}

var redisSpecs = []Spec{
	{Name: "maxmemory", Kind: KindSize, Default: "0", Min: 0, Max: 1024 * gib, MemoryCap: true, Description: "Memory limit for keys, 0 is no limit."},
	{Name: "maxmemory-policy", Kind: KindEnum, Default: "noeviction", Options: []string{"noeviction", "allkeys-lru", "allkeys-lfu", "allkeys-random", "volatile-lru", "volatile-lfu", "volatile-random", "volatile-ttl"}, Description: "What to evict when maxmemory is reached."},
	{Name: "maxclients", Kind: KindInt, Default: "10000", Min: 1, Max: 100000, ConnectionCap: true, Description: "Concurrent client connections."},
	{Name: "timeout", Kind: KindInt, Default: "0", Min: 0, Max: 86400, Description: "Seconds before an idle client is closed, 0 is never."},
	{Name: "tcp-keepalive", Kind: KindInt, Default: "300", Min: 0, Max: 7200, Description: "Seconds between TCP keepalive probes."},
	{Name: "hz", Kind: KindInt, Default: "10", Min: 1, Max: 500, Description: "Background task frequency."},
	{Name: "appendfsync", Kind: KindEnum, Default: "everysec", Options: []string{"always", "everysec", "no"}, Description: "How often the append only file is synced."},
	{Name: "lazyfree-lazy-eviction", Kind: KindBool, Default: "no", Options: []string{"yes", "no"}, Description: "Free evicted keys in the background."},
}

var mongoSpecs = []Spec{
	{Name: "wiredTigerCacheSizeGB", Kind: KindFloat, Default: "", Min: 0.25, Max: 10000, MemoryCap: true, Description: "WiredTiger cache size in GB, empty uses the engine default."},
	{Name: "maxConns", Kind: KindInt, Default: "1000000", Min: 1, Max: 1000000, ConnectionCap: true, Description: "Concurrent client connections."},
	{Name: "slowms", Kind: KindInt, Default: "100", Min: 0, Max: 3600000, Description: "Milliseconds above which an operation is slow."},
}

// Denied names are refused anyway by the allowlist; listing them gives a
// clearer error for settings that change security posture or file access.
var denied = map[Family][]string{
	FamilyPostgres: {"listen_addresses", "port", "ssl", "ssl_cert_file", "ssl_key_file", "ssl_ca_file", "archive_command", "archive_mode", "restore_command", "shared_preload_libraries", "local_preload_libraries", "session_preload_libraries", "include", "include_dir", "include_if_exists", "hba_file", "ident_file", "data_directory", "config_file", "log_directory", "unix_socket_directories", "password_encryption", "log_min_duration_statement", "wal_level", "dynamic_library_path"},
	FamilyMySQL:    {"local_infile", "secure_file_priv", "bind_address", "bind-address", "port", "datadir", "plugin_load", "plugin_load_add", "plugin_dir", "init_file", "init_connect", "skip_grant_tables", "skip-grant-tables", "ssl_cert", "ssl_key", "ssl_ca", "require_secure_transport", "log_bin", "general_log_file", "slow_query_log_file", "log_error", "tmpdir", "default_authentication_plugin", "sql_mode", "slow_query_log", "long_query_time", "log_output"},
	FamilyRedis:    {"requirepass", "masterauth", "rename-command", "bind", "port", "protected-mode", "dir", "dbfilename", "include", "loadmodule", "aclfile", "tls-port", "tls-cert-file", "tls-key-file", "unixsocket", "enable-debug-command", "enable-module-command", "replicaof", "slaveof", "databases", "appendonly", "appenddirname", "appendfilename"},
	FamilyMongo:    {"bind_ip", "bind_ip_all", "port", "auth", "noauth", "keyFile", "tlsMode", "tlsCertificateKeyFile", "dbpath", "logpath", "setParameter", "config", "sslMode", "enableLocalhostAuthBypass"},
}

// FamilyFor maps an engine to its parameter family; ok is false when the
// engine takes no tuning parameters.
func FamilyFor(engine string) (Family, bool) {
	switch engine {
	case "postgres":
		return FamilyPostgres, true
	case "mysql", "mariadb":
		return FamilyMySQL, true
	case "redis", "valkey":
		return FamilyRedis, true
	case "mongodb":
		return FamilyMongo, true
	}
	return "", false
}

// Specs returns the allowlist for engine, nil when unsupported.
func Specs(engine string) []Spec {
	f, ok := FamilyFor(engine)
	if !ok {
		return nil
	}
	switch f {
	case FamilyPostgres:
		return postgresSpecs
	case FamilyMySQL:
		return mysqlSpecs
	case FamilyRedis:
		return redisSpecs
	default:
		return mongoSpecs
	}
}

// Denied returns the explicitly refused names for engine.
func Denied(engine string) []string {
	f, _ := FamilyFor(engine)
	return denied[f]
}

// ErrUnsupportedEngine is returned for an engine with no allowlist.
var ErrUnsupportedEngine = errors.New("this engine takes no tuning parameters")

func lookup(engine, name string) (Spec, bool) {
	for _, s := range Specs(engine) {
		if s.Name == name {
			return s, true
		}
	}
	return Spec{}, false
}

// Validate checks every name and value and returns the canonical form.
func Validate(engine string, params map[string]string, lim Limits) (map[string]string, error) {
	if _, ok := FamilyFor(engine); !ok {
		return nil, ErrUnsupportedEngine
	}
	out := make(map[string]string, len(params))
	for _, name := range sortedKeys(params) {
		v, err := ValidateOne(engine, name, params[name], lim)
		if err != nil {
			return nil, err
		}
		out[name] = v
	}
	return out, nil
}

// ValidateOne checks one parameter and returns its canonical value.
func ValidateOne(engine, name, value string, lim Limits) (string, error) {
	s, ok := lookup(engine, name)
	if !ok {
		for _, d := range Denied(engine) {
			if strings.EqualFold(d, name) {
				return "", fmt.Errorf("parameter %q is managed by the platform and cannot be set", name)
			}
		}
		return "", fmt.Errorf("parameter %q is not in the %s allowlist", safeName(name), engine)
	}
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 32 || !plainToken(value) {
		return "", fmt.Errorf("parameter %q: value must be a plain number, size or option", name)
	}
	fam, _ := FamilyFor(engine)
	switch s.Kind {
	case KindInt:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return "", fmt.Errorf("parameter %q: %q is not a whole number", name, value)
		}
		if err := inRange(s, float64(n), lim); err != nil {
			return "", fmt.Errorf("parameter %q: %w", name, err)
		}
		return strconv.FormatInt(n, 10), nil
	case KindFloat:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return "", fmt.Errorf("parameter %q: %q is not a number", name, value)
		}
		if err := inRange(s, f, lim); err != nil {
			return "", fmt.Errorf("parameter %q: %w", name, err)
		}
		return strconv.FormatFloat(f, 'f', -1, 64), nil
	case KindSize:
		n, unit, bytes, err := parseSize(fam, value)
		if err != nil {
			return "", fmt.Errorf("parameter %q: %w", name, err)
		}
		if err := inRange(s, float64(bytes), lim); err != nil {
			return "", fmt.Errorf("parameter %q: %w", name, err)
		}
		return strconv.FormatInt(n, 10) + unit, nil
	case KindBool:
		b, ok := parseBool(value)
		if !ok {
			return "", fmt.Errorf("parameter %q: %q is not on or off", name, value)
		}
		if b {
			return s.Options[0], nil
		}
		return s.Options[1], nil
	default:
		for _, o := range s.Options {
			if o == value {
				return o, nil
			}
		}
		return "", fmt.Errorf("parameter %q: %q is not one of %s", name, value, strings.Join(s.Options, ", "))
	}
}

// safeName keeps an echoed unknown name printable and short.
func safeName(name string) string {
	if len(name) > 64 || !plainToken(name) {
		return "(invalid name)"
	}
	return name
}

// plainToken refuses quotes, separators, whitespace and escapes so a value
// cannot carry a second directive even before the typed parse.
func plainToken(v string) bool {
	for _, r := range v {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_'
		if !ok {
			return false
		}
	}
	return true
}

func inRange(s Spec, v float64, lim Limits) error {
	maxV := s.Max
	if s.ConnectionCap && lim.MaxConnections > 0 && float64(lim.MaxConnections) < maxV {
		maxV = float64(lim.MaxConnections)
	}
	if s.MemoryCap && lim.MaxMemoryBytes > 0 {
		// The float memory parameter is in GB, the size ones in bytes.
		ceil := float64(lim.MaxMemoryBytes)
		if s.Kind == KindFloat {
			ceil /= gib
		}
		maxV = math.Min(maxV, ceil)
	}
	if v < s.Min || v > maxV {
		if s.Kind == KindSize {
			return fmt.Errorf("must be between %s and %s", humanBytes(s.Min), humanBytes(maxV))
		}
		return fmt.Errorf("must be between %s and %s", fmtNum(s.Min), fmtNum(maxV))
	}
	return nil
}

func fmtNum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func humanBytes(b float64) string {
	switch {
	case b >= gib && math.Mod(b, gib) == 0:
		return fmtNum(b/gib) + "GB"
	case b >= mib && math.Mod(b, mib) == 0:
		return fmtNum(b/mib) + "MB"
	case b >= kib && math.Mod(b, kib) == 0:
		return fmtNum(b/kib) + "kB"
	}
	return fmtNum(b) + "B"
}

type unitDef struct {
	spelling string
	mult     int64
}

// sizeUnits lists accepted suffixes per family, written back in the spelling each engine parses.
func sizeUnits(f Family) map[string]unitDef {
	switch f {
	case FamilyPostgres:
		return map[string]unitDef{"kb": {"kB", kib}, "mb": {"MB", mib}, "gb": {"GB", gib}, "tb": {"TB", 1024 * gib}}
	case FamilyMySQL:
		return map[string]unitDef{"k": {"K", kib}, "m": {"M", mib}, "g": {"G", gib}, "kb": {"K", kib}, "mb": {"M", mib}, "gb": {"G", gib}}
	default:
		return map[string]unitDef{"": {"", 1}, "b": {"", 1}, "kb": {"kb", kib}, "mb": {"mb", mib}, "gb": {"gb", gib}}
	}
}

func parseSize(f Family, v string) (int64, string, int64, error) {
	i := 0
	for i < len(v) && v[i] >= '0' && v[i] <= '9' {
		i++
	}
	if i == 0 || i > 15 {
		return 0, "", 0, fmt.Errorf("%q is not a size like 256MB", v)
	}
	n, err := strconv.ParseInt(v[:i], 10, 64)
	if err != nil {
		return 0, "", 0, fmt.Errorf("%q is not a size like 256MB", v)
	}
	u, ok := sizeUnits(f)[strings.ToLower(v[i:])]
	if !ok {
		return 0, "", 0, fmt.Errorf("%q has an unknown size unit", v)
	}
	if n > math.MaxInt64/u.mult {
		return 0, "", 0, fmt.Errorf("%q is too large", v)
	}
	return n, u.spelling, n * u.mult, nil
}

func parseBool(v string) (bool, bool) {
	switch strings.ToLower(v) {
	case "on", "true", "yes", "1":
		return true, true
	case "off", "false", "no", "0":
		return false, true
	}
	return false, false
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Args renders applied parameters as separate argv elements. Entries that
// fail validation are dropped, so a stored row edited by hand never reaches the engine.
func Args(engine string, params map[string]string, lim Limits) []string {
	f, ok := FamilyFor(engine)
	if !ok || len(params) == 0 {
		return nil
	}
	var out []string
	for _, k := range sortedKeys(params) {
		v, err := ValidateOne(engine, k, params[k], lim)
		if err != nil {
			continue
		}
		switch f {
		case FamilyPostgres:
			out = append(out, "-c", k+"="+v)
		case FamilyMySQL:
			out = append(out, "--"+k+"="+v)
		default:
			out = append(out, "--"+k, v)
		}
	}
	return out
}

// Fingerprint is a short hash of the rendered args, "" when there are none,
// so a database without parameters keeps its existing container label set.
func Fingerprint(args []string) string {
	if len(args) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(args, "\x00")))
	return hex.EncodeToString(sum[:8])
}
