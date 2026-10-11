package telemetry

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Field filter operators.
const (
	OpEqual    = "="
	OpNotEqual = "!="
	OpContains = "~"
	OpGT       = ">"
	OpGTE      = ">="
	OpLT       = "<"
	OpLTE      = "<="
)

// FieldFilter matches one key of a structured (JSON) log line. Path may be
// dotted to reach nested objects.
type FieldFilter struct {
	Path  string
	Op    string
	Value string
}

// maxFieldFilters bounds how many filters one request may carry.
const maxFieldFilters = 8

// opOrder lists operators longest first so ">=" is not read as ">".
var opOrder = []string{OpNotEqual, OpGTE, OpLTE, OpEqual, OpContains, OpGT, OpLT}

// ParseFieldFilter parses "key=value", "key!=value", "key~text", "key>5".
func ParseFieldFilter(s string) (FieldFilter, error) {
	s = strings.TrimSpace(s)
	best := -1
	var bestOp string
	for _, op := range opOrder {
		if i := strings.Index(s, op); i > 0 && (best < 0 || i < best || (i == best && len(op) > len(bestOp))) {
			best, bestOp = i, op
		}
	}
	if best < 0 {
		return FieldFilter{}, fmt.Errorf("field filter %q must look like key=value, key!=value, key~text or key>number", s)
	}
	path := strings.TrimSpace(s[:best])
	val := strings.TrimSpace(s[best+len(bestOp):])
	if path == "" {
		return FieldFilter{}, errors.New("field filter key must not be empty")
	}
	if bestOp == OpGT || bestOp == OpGTE || bestOp == OpLT || bestOp == OpLTE {
		if _, err := strconv.ParseFloat(val, 64); err != nil {
			return FieldFilter{}, fmt.Errorf("field filter %q needs a number after %s", s, bestOp)
		}
	}
	return FieldFilter{Path: path, Op: bestOp, Value: val}, nil
}

// ParseFieldFilters parses several filters, rejecting more than the cap.
func ParseFieldFilters(raw []string) ([]FieldFilter, error) {
	if len(raw) > maxFieldFilters {
		return nil, fmt.Errorf("at most %d field filters are allowed", maxFieldFilters)
	}
	out := make([]FieldFilter, 0, len(raw))
	for _, r := range raw {
		f, err := ParseFieldFilter(r)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func lookupPath(m map[string]json.RawMessage, path string) (json.RawMessage, bool) {
	cur := m
	parts := strings.Split(path, ".")
	for i, p := range parts {
		raw, ok := cur[p]
		if !ok {
			return nil, false
		}
		if i == len(parts)-1 {
			return raw, true
		}
		var next map[string]json.RawMessage
		if err := json.Unmarshal(raw, &next); err != nil {
			return nil, false
		}
		cur = next
	}
	return nil, false
}

func rawString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.TrimSpace(string(raw))
}

// MatchFieldFilters reports whether a line satisfies every filter. A line that is not
// structured, or lacks the key, matches only the != operator.
func MatchFieldFilters(e LogEntry, filters []FieldFilter) bool {
	if len(filters) == 0 {
		return true
	}
	var fields map[string]json.RawMessage
	if e.Structured && e.FieldsJSON != "" {
		if err := json.Unmarshal([]byte(e.FieldsJSON), &fields); err != nil {
			fields = nil
		}
	}
	for _, f := range filters {
		raw, ok := lookupPath(fields, f.Path)
		if !ok {
			if f.Op != OpNotEqual {
				return false
			}
			continue
		}
		got := rawString(raw)
		switch f.Op {
		case OpEqual:
			if got != f.Value {
				return false
			}
		case OpNotEqual:
			if got == f.Value {
				return false
			}
		case OpContains:
			if !strings.Contains(strings.ToLower(got), strings.ToLower(f.Value)) {
				return false
			}
		default:
			n, err := strconv.ParseFloat(got, 64)
			want, _ := strconv.ParseFloat(f.Value, 64)
			if err != nil || !cmpNumber(f.Op, n, want) {
				return false
			}
		}
	}
	return true
}

func cmpNumber(op string, got, want float64) bool {
	switch op {
	case OpGT:
		return got > want
	case OpGTE:
		return got >= want
	case OpLT:
		return got < want
	case OpLTE:
		return got <= want
	}
	return false
}
