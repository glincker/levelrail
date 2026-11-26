package dbviewer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// RedisKey is one scanned key with its type and TTL (seconds, -1 for none).
type RedisKey struct {
	Key  string `json:"key"`
	Type string `json:"type"`
	TTL  int64  `json:"ttl"`
}

// RedisScan is one SCAN step. Cursor "0" means the scan is complete.
type RedisScan struct {
	Cursor string     `json:"cursor"`
	Keys   []RedisKey `json:"keys"`
}

// RedisValue is a key's value rendered as text lines, capped by Limits.
type RedisValue struct {
	Type      string   `json:"type"`
	TTL       int64    `json:"ttl"`
	Lines     []string `json:"lines"`
	Truncated bool     `json:"truncated"`
	// Typed detail (redis_typed.go). Size and MemoryBytes are -1 when unknown.
	Encoding    string       `json:"encoding"`
	Size        int64        `json:"size"`
	MemoryBytes int64        `json:"memoryBytes"`
	Subtype     string       `json:"subtype,omitempty"`
	BitLength   int64        `json:"bitLength,omitempty"`
	BitCount    int64        `json:"bitCount,omitempty"`
	Unsupported bool         `json:"unsupported,omitempty"`
	Entries     []RedisEntry `json:"entries"`
}

// redisCmdScript mirrors the dump path's TLS probe so a TLS-only Redis works.
const redisCmdScript = `RTLS=""; [ -f /certs/tls.crt ] && RTLS="--tls --insecure -p 6380"; BIN=$(command -v redis-cli || command -v keydb-cli); exec "$BIN" $RTLS "$@"`

func redisArgs(args ...string) []string {
	return append([]string{"sh", "-c", redisCmdScript, "sh"}, args...)
}

func redisQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func (t Target) redisRun(ctx context.Context, stdin string, args ...string) ([]string, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, t.Limits.Timeout)
	defer cancel()
	rc, err := t.Exec.ExecWithInput(ctx, t.ContainerID, redisArgs(args...), strings.NewReader(stdin))
	if err != nil {
		return nil, false, wrapExecError(ctx, err)
	}
	defer func() { _ = rc.Close() }()

	capped := &capReader{r: rc, left: t.Limits.MaxBytes}
	sc := bufio.NewScanner(capped)
	sc.Buffer(make([]byte, 0, 64<<10), t.Limits.MaxCellBytes+1024)
	var lines []string
	truncated := false
	for sc.Scan() {
		if len(lines) >= t.Limits.MaxRows*3 {
			truncated = true
			break
		}
		lines = append(lines, truncateCell(sc.Text(), t.Limits.MaxCellBytes))
	}
	if capped.tripd {
		truncated = true
	}
	if err := sc.Err(); err != nil && !capped.tripd {
		return nil, false, wrapExecError(ctx, err)
	}
	if !truncated {
		if _, err := io.Copy(io.Discard, rc); err != nil {
			return nil, false, wrapExecError(ctx, err)
		}
	}
	return lines, truncated, nil
}

// ScanKeys runs one SCAN step and looks up each key's type and TTL.
func (t Target) ScanKeys(ctx context.Context, cursor, pattern string, count int) (RedisScan, error) {
	if _, err := strconv.ParseUint(cursor, 10, 64); err != nil {
		cursor = "0"
	}
	if pattern == "" {
		pattern = "*"
	}
	if count <= 0 || count > t.Limits.MaxRows {
		count = min(100, t.Limits.MaxRows)
	}
	lines, _, err := t.redisLeaves(ctx, "SCAN "+cursor+" MATCH "+redisQuote(pattern)+" COUNT "+strconv.Itoa(count)+"\n")
	if err != nil {
		return RedisScan{}, err
	}
	out := RedisScan{Cursor: "0", Keys: []RedisKey{}}
	if len(lines) == 0 {
		return out, nil
	}
	out.Cursor = strings.TrimSpace(lines[0])
	keys := lines[1:]
	if len(keys) == 0 {
		return out, nil
	}
	var script strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&script, "TYPE %s\nTTL %s\n", redisQuote(k), redisQuote(k))
	}
	meta, _, err := t.redisLeaves(ctx, script.String())
	if err != nil {
		return RedisScan{}, err
	}
	for i, k := range keys {
		rk := RedisKey{Key: k, Type: "unknown", TTL: -1}
		if 2*i+1 < len(meta) {
			rk.Type = strings.TrimSpace(meta[2*i])
			rk.TTL, _ = strconv.ParseInt(strings.TrimSpace(meta[2*i+1]), 10, 64)
		}
		out.Keys = append(out.Keys, rk)
	}
	return out, nil
}
