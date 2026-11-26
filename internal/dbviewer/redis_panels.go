package dbviewer

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

// RedisInfoField is one allow listed INFO value.
type RedisInfoField struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// RedisInfoSection is one INFO section with only its allow listed fields.
type RedisInfoSection struct {
	Name   string           `json:"name"`
	Fields []RedisInfoField `json:"fields"`
}

// RedisInfo is the INFO panel. Paths, run ids and anything secret-shaped
// are never in the allow list.
type RedisInfo struct {
	Sections []RedisInfoSection `json:"sections"`
}

// RedisSlowEntry never carries arguments: only the command name, how many
// arguments there were and their total size.
type RedisSlowEntry struct {
	ID         int64  `json:"id"`
	Time       int64  `json:"time"`
	DurationUS int64  `json:"durationUs"`
	Command    string `json:"command"`
	ArgCount   int    `json:"argCount"`
	Client     string `json:"client,omitempty"`
}

// RedisSlowlog is the capped SLOWLOG GET result.
type RedisSlowlog struct {
	Entries   []RedisSlowEntry `json:"entries"`
	Truncated bool             `json:"truncated"`
}

// RedisClient is one connected client. Names can hold secrets, so they are
// not returned.
type RedisClient struct {
	ID    string `json:"id"`
	Addr  string `json:"addr"`
	Age   int64  `json:"age"`
	Idle  int64  `json:"idle"`
	Flags string `json:"flags"`
	DB    string `json:"db"`
	Cmd   string `json:"cmd"`
	User  string `json:"user"`
}

// RedisClients is the capped CLIENT LIST result.
type RedisClients struct {
	Clients   []RedisClient `json:"clients"`
	Truncated bool          `json:"truncated"`
}

var redisInfoSections = []string{"server", "clients", "memory", "stats", "replication", "keyspace", "persistence"}

var redisInfoAllow = map[string]map[string]bool{
	"server":      setOf("redis_version", "valkey_version", "redis_mode", "arch_bits", "tcp_port", "uptime_in_seconds", "uptime_in_days", "hz", "configured_hz", "io_threads_active"),
	"clients":     setOf("connected_clients", "blocked_clients", "maxclients", "cluster_connections", "tracking_clients"),
	"memory":      setOf("used_memory", "used_memory_human", "used_memory_rss", "used_memory_rss_human", "used_memory_peak", "used_memory_peak_human", "used_memory_dataset", "maxmemory", "maxmemory_human", "maxmemory_policy", "mem_fragmentation_ratio", "total_system_memory_human"),
	"stats":       setOf("total_connections_received", "total_commands_processed", "instantaneous_ops_per_sec", "total_net_input_bytes", "total_net_output_bytes", "rejected_connections", "expired_keys", "evicted_keys", "keyspace_hits", "keyspace_misses", "total_error_replies"),
	"replication": setOf("role", "connected_slaves", "master_host", "master_port", "master_link_status", "master_last_io_seconds_ago", "master_repl_offset", "repl_backlog_active"),
	"persistence": setOf("loading", "rdb_changes_since_last_save", "rdb_bgsave_in_progress", "rdb_last_save_time", "rdb_last_bgsave_status", "aof_enabled", "aof_rewrite_in_progress", "aof_last_write_status"),
}

var redisKeyspaceKey = regexp.MustCompile(`^db\d+$`)

func setOf(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// ParseRedisInfo splits INFO output into allow listed sections.
func ParseRedisInfo(lines []string) RedisInfo {
	out := RedisInfo{Sections: []RedisInfoSection{}}
	cur := -1
	for _, raw := range lines {
		l := strings.TrimSpace(raw)
		if l == "" {
			continue
		}
		if name, ok := strings.CutPrefix(l, "# "); ok {
			name = strings.ToLower(name)
			cur = -1
			for _, s := range redisInfoSections {
				if s == name {
					out.Sections = append(out.Sections, RedisInfoSection{Name: name, Fields: []RedisInfoField{}})
					cur = len(out.Sections) - 1
				}
			}
			continue
		}
		k, v, ok := strings.Cut(l, ":")
		if !ok || cur < 0 {
			continue
		}
		sec := &out.Sections[cur]
		if redisInfoAllow[sec.Name][k] || (sec.Name == "keyspace" && redisKeyspaceKey.MatchString(k)) {
			sec.Fields = append(sec.Fields, RedisInfoField{Key: k, Value: truncateCell(v, 256)})
		}
	}
	return out
}

// Info runs fixed INFO section commands.
func (t Target) Info(ctx context.Context) (RedisInfo, error) {
	var sb strings.Builder
	for _, s := range redisInfoSections {
		sb.WriteString("INFO " + s + "\n")
	}
	lines, _, err := t.redisRun(ctx, sb.String())
	if err != nil {
		return RedisInfo{}, err
	}
	return ParseRedisInfo(lines), nil
}

var slowLabel = regexp.MustCompile(`^(\d+)\) `)

// ParseRedisSlowlog reads `redis-cli --no-raw SLOWLOG GET` output. Argument
// values are never collected: only the command name (the first element of
// field 4) and the number of further argument lines are kept. Labels are
// right aligned past nine items, so lines are read by trimmed position.
func ParseRedisSlowlog(lines []string) []RedisSlowEntry {
	out := []RedisSlowEntry{}
	var cur *RedisSlowEntry
	fieldCol := -1
	finish := func() {
		if cur != nil {
			out = append(out, *cur)
		}
		cur, fieldCol = nil, -1
	}
	for _, l := range lines {
		trimmed := strings.TrimLeft(l, " ")
		indent := len(l) - len(trimmed)
		m := slowLabel.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		rest := trimmed[len(m[0]):]
		if inner, ok := strings.CutPrefix(rest, "1) (integer) "); ok {
			finish()
			cur = &RedisSlowEntry{ID: parseIntOr(inner, 0)}
			continue
		}
		if cur == nil {
			continue
		}
		if fieldCol < 0 {
			fieldCol = indent
		}
		if indent > fieldCol {
			cur.ArgCount++
			continue
		}
		switch m[1] {
		case "2":
			cur.Time = parseIntOr(slowValue(rest), 0)
		case "3":
			cur.DurationUS = parseIntOr(slowValue(rest), 0)
		case "4":
			if first, ok := strings.CutPrefix(rest, "1) "); ok {
				cur.Command = strings.ToUpper(truncateCell(slowValue(first), 32))
			}
		case "5":
			cur.Client = truncateCell(slowValue(rest), 64)
		}
	}
	finish()
	return out
}

func slowValue(s string) string {
	s = strings.TrimPrefix(s, "(integer) ")
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// Slowlog reads the newest count entries (clamped to the row cap).
func (t Target) Slowlog(ctx context.Context, count int) (RedisSlowlog, error) {
	if count <= 0 || count > t.Limits.MaxRows {
		count = t.Limits.MaxRows
	}
	lines, trunc, err := t.redisRun(ctx, "SLOWLOG GET "+strconv.Itoa(count)+"\n", "--no-raw")
	if err != nil {
		return RedisSlowlog{}, err
	}
	return RedisSlowlog{Entries: ParseRedisSlowlog(lines), Truncated: trunc}, nil
}

// ParseRedisClients reads CLIENT LIST lines, keeping only safe fields.
func ParseRedisClients(lines []string) []RedisClient {
	out := []RedisClient{}
	for _, l := range lines {
		f := map[string]string{}
		for _, tok := range strings.Fields(l) {
			if k, v, ok := strings.Cut(tok, "="); ok {
				f[k] = v
			}
		}
		if f["id"] == "" {
			continue
		}
		out = append(out, RedisClient{
			ID: f["id"], Addr: f["addr"], Age: parseIntOr(f["age"], 0), Idle: parseIntOr(f["idle"], 0),
			Flags: f["flags"], DB: f["db"], Cmd: f["cmd"], User: f["user"],
		})
	}
	return out
}

// Clients lists connected clients, capped by the row limit.
func (t Target) Clients(ctx context.Context) (RedisClients, error) {
	lines, trunc, err := t.redisRun(ctx, "CLIENT LIST\n")
	if err != nil {
		return RedisClients{}, err
	}
	cs := ParseRedisClients(lines)
	if len(cs) > t.Limits.MaxRows {
		cs, trunc = cs[:t.Limits.MaxRows], true
	}
	return RedisClients{Clients: cs, Truncated: trunc}, nil
}
