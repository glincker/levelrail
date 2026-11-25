package dbviewer

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

// RedisPair is one field and value of a stream entry.
type RedisPair struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// RedisEntry is one element of a typed value: a list item, set member, hash
// field, sorted set member with score, or stream entry with its pairs.
type RedisEntry struct {
	ID    string      `json:"id,omitempty"`
	Field string      `json:"field,omitempty"`
	Value string      `json:"value,omitempty"`
	Score string      `json:"score,omitempty"`
	Pairs []RedisPair `json:"pairs,omitempty"`
}

const redisJSONType = "ReJSON-RL"

func parseIntOr(s string, def int64) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return def
	}
	return n
}

func lineAt(lines []string, i int) string {
	if i < len(lines) {
		return strings.TrimSpace(lines[i])
	}
	return ""
}

// redisSizeCmd is the fixed size command per type; empty when none applies.
func redisSizeCmd(typ string) string {
	switch typ {
	case "string":
		return "STRLEN"
	case "hash":
		return "HLEN"
	case "list":
		return "LLEN"
	case "set":
		return "SCARD"
	case "zset":
		return "ZCARD"
	case "stream":
		return "XLEN"
	}
	return ""
}

func pairsOf(lines []string) [][2]string {
	out := make([][2]string, 0, len(lines)/2)
	for i := 0; i+1 < len(lines); i += 2 {
		out = append(out, [2]string{lines[i], lines[i+1]})
	}
	return out
}

var nrLabel = regexp.MustCompile(`^(\d+)\) `)

// nrLine is one decoded --no-raw line: Col1 is where the first label's
// content starts, which is equal for every top-level item even though
// redis-cli right-aligns labels.
type nrLine struct {
	Col1 int
	Leaf string
	Skip bool
}

func unescapeRepr(s string) string {
	s = strings.TrimPrefix(s, `"`)
	s = strings.TrimSuffix(s, `"`)
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'b':
			b.WriteByte('\b')
		case 'a':
			b.WriteByte(7)
		case 'x':
			if i+3 <= len(s) {
				if n, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
					b.WriteByte(byte(n))
					i += 2
					break
				}
			}
			b.WriteByte('x')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// parseNoRaw decodes `redis-cli --no-raw` lines. Every string is quoted and
// escaped, so a value can never be mistaken for structure.
func parseNoRaw(lines []string) []nrLine {
	out := make([]nrLine, 0, len(lines))
	for _, l := range lines {
		trimmed := strings.TrimLeft(l, " ")
		indent := len(l) - len(trimmed)
		col1 := indent
		if m := nrLabel.FindString(trimmed); m != "" {
			col1 = indent + len(m)
		}
		rest := trimmed
		for {
			m := nrLabel.FindString(rest)
			if m == "" {
				break
			}
			rest = rest[len(m):]
		}
		nl := nrLine{Col1: col1}
		switch {
		case strings.HasPrefix(rest, `"`):
			nl.Leaf = unescapeRepr(rest)
		case strings.HasPrefix(rest, "(integer) "):
			nl.Leaf = strings.TrimPrefix(rest, "(integer) ")
		case rest == "(nil)":
			nl.Leaf = ""
		case strings.HasPrefix(rest, "(empty "):
			nl.Skip = true
		case rest == "":
			nl.Skip = true
		default:
			nl.Leaf = strings.TrimPrefix(rest, "(error) ")
		}
		out = append(out, nl)
	}
	return out
}

func leavesOf(lines []string) []string {
	out := []string{}
	for _, n := range parseNoRaw(lines) {
		if !n.Skip {
			out = append(out, n.Leaf)
		}
	}
	return out
}

// redisLeaves runs the script in --no-raw mode and returns decoded leaves.
func (t Target) redisLeaves(ctx context.Context, script string) ([]string, bool, error) {
	lines, trunc, err := t.redisRun(ctx, script, "--no-raw")
	if err != nil {
		return nil, false, err
	}
	return leavesOf(lines), trunc, nil
}

// parseStreamNoRaw reads XRANGE output: top-level lines start an entry, all
// deeper leaves are its alternating fields and values.
func parseStreamNoRaw(lines []string) []RedisEntry {
	out := []RedisEntry{}
	ns := parseNoRaw(lines)
	if len(ns) == 0 {
		return out
	}
	top := ns[0].Col1
	var rest []string
	flush := func() {
		if len(out) == 0 {
			return
		}
		for _, p := range pairsOf(rest) {
			out[len(out)-1].Pairs = append(out[len(out)-1].Pairs, RedisPair{Field: p[0], Value: p[1]})
		}
		rest = nil
	}
	for _, n := range ns {
		if n.Skip {
			continue
		}
		if n.Col1 == top {
			flush()
			out = append(out, RedisEntry{ID: n.Leaf})
			continue
		}
		rest = append(rest, n.Leaf)
	}
	flush()
	return out
}

func typedEntries(typ string, leaves []string) []RedisEntry {
	out := []RedisEntry{}
	switch typ {
	case "list", "set":
		for _, l := range leaves {
			out = append(out, RedisEntry{Value: l})
		}
	case "hash":
		for _, p := range pairsOf(leaves) {
			out = append(out, RedisEntry{Field: p[0], Value: p[1]})
		}
	case "zset":
		for _, p := range pairsOf(leaves) {
			out = append(out, RedisEntry{Value: p[0], Score: p[1]})
		}
	}
	return out
}

// readCount bounds how many elements are read so that the estimated reply
// (average element size times count) stays within the byte cap.
func (t Target) readCount(v RedisValue) int {
	n := t.Limits.MaxRows
	if v.Size > 0 && v.MemoryBytes > 0 {
		avg := v.MemoryBytes/v.Size + 1
		if fit := int64(t.Limits.MaxBytes) / avg; fit < int64(n) {
			n = int(max(fit, 1))
		}
	}
	return n
}

// GetKey reads one key with fixed read-only commands chosen by type. The key
// is only ever sent through redisQuote, and every reply is bounded before it
// is read.
func (t Target) GetKey(ctx context.Context, key string) (RedisValue, error) {
	q := redisQuote(key)
	meta, _, err := t.redisLeaves(ctx, "TYPE "+q+"\nTTL "+q+"\nOBJECT ENCODING "+q+"\nMEMORY USAGE "+q+"\n")
	if err != nil {
		return RedisValue{}, err
	}
	if len(meta) < 2 || lineAt(meta, 0) == "none" {
		return RedisValue{}, ErrNotFound
	}
	v := RedisValue{
		Type: lineAt(meta, 0), Lines: []string{}, Entries: []RedisEntry{},
		TTL: parseIntOr(lineAt(meta, 1), -1), Encoding: lineAt(meta, 2),
		MemoryBytes: parseIntOr(lineAt(meta, 3), -1), Size: -1,
	}

	if sc := redisSizeCmd(v.Type); sc != "" {
		sz, _, err := t.redisLeaves(ctx, sc+" "+q+"\n")
		if err != nil {
			return RedisValue{}, err
		}
		v.Size = parseIntOr(lineAt(sz, 0), -1)
	}

	if v.Type == redisJSONType {
		return t.readJSON(ctx, v, q)
	}
	maxBytes := int64(t.Limits.MaxBytes)
	n := strconv.Itoa(t.readCount(v) - 1)
	var cmd string
	switch v.Type {
	case "string":
		cmd = "GET " + q
		if v.Size > maxBytes {
			cmd = "GETRANGE " + q + " 0 " + strconv.FormatInt(maxBytes-1, 10)
			v.Truncated = true
		}
	case "list":
		cmd = "LRANGE " + q + " 0 " + n
	case "set":
		cmd = "SSCAN " + q + " 0 COUNT " + n
	case "hash":
		cmd = "HSCAN " + q + " 0 COUNT " + n
	case "zset":
		cmd = "ZRANGE " + q + " 0 " + n + " WITHSCORES"
	case "stream":
		return t.readStream(ctx, v, q)
	default:
		v.Lines = []string{"(" + v.Type + " values are not shown)"}
		return v, nil
	}
	leaves, trunc, err := t.redisLeaves(ctx, cmd+"\n")
	if err != nil {
		return RedisValue{}, err
	}
	if (v.Type == "set" || v.Type == "hash") && len(leaves) > 0 {
		leaves = leaves[1:]
	}
	v.Lines = leaves
	got := int64(len(leaves))
	if v.Type == "hash" || v.Type == "zset" {
		got /= 2
	}
	v.Truncated = v.Truncated || trunc || (v.Type != "string" && v.Size > got)
	v.Entries = typedEntries(v.Type, leaves)
	if v.Type == "string" {
		t.annotateString(ctx, &v, q)
	}
	return v, nil
}

func (t Target) readStream(ctx context.Context, v RedisValue, q string) (RedisValue, error) {
	n := t.readCount(v)
	lines, trunc, err := t.redisRun(ctx, "XRANGE "+q+" - + COUNT "+strconv.Itoa(n)+"\n", "--no-raw")
	if err != nil {
		return RedisValue{}, err
	}
	v.Entries = parseStreamNoRaw(lines)
	v.Lines = leavesOf(lines)
	v.Truncated = trunc || v.Size > int64(len(v.Entries))
	return v, nil
}

// readJSON asks JSON.DEBUG MEMORY first so a huge document is never fetched.
func (t Target) readJSON(ctx context.Context, v RedisValue, q string) (RedisValue, error) {
	mem, _, err := t.redisLeaves(ctx, "JSON.DEBUG MEMORY "+q+"\n")
	if err != nil {
		return RedisValue{}, err
	}
	first := lineAt(mem, 0)
	if strings.Contains(first, "unknown command") {
		v.Unsupported = true
		v.Lines = []string{"(the JSON module is not available on this server)"}
		return v, nil
	}
	if parseIntOr(first, 0) > int64(t.Limits.MaxBytes) {
		v.Truncated = true
		v.Lines = []string{"(document larger than the viewer limit, not fetched)"}
		return v, nil
	}
	lines, trunc, err := t.redisLeaves(ctx, "JSON.GET "+q+"\n")
	if err != nil {
		return RedisValue{}, err
	}
	if len(lines) > 0 && strings.Contains(lines[0], "unknown command") {
		v.Unsupported = true
		v.Lines = []string{"(the JSON module is not available on this server)"}
		return v, nil
	}
	v.Lines, v.Truncated = lines, trunc
	return v, nil
}

// annotateString marks HyperLogLog sketches and reports bit stats for
// strings small enough to count within the byte cap.
func (t Target) annotateString(ctx context.Context, v *RedisValue, q string) {
	if len(v.Lines) > 0 && strings.HasPrefix(v.Lines[0], "HYLL") {
		v.Subtype = "hyperloglog"
		v.Lines = []string{"(hyperloglog sketch, binary)"}
		return
	}
	if v.Size < 0 || v.Size > int64(t.Limits.MaxBytes) {
		return
	}
	v.BitLength = v.Size * 8
	if bc, _, err := t.redisLeaves(ctx, "BITCOUNT "+q+"\n"); err == nil {
		v.BitCount = parseIntOr(lineAt(bc, 0), 0)
	}
}
