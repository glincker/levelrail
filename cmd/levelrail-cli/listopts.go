package main

import (
	"encoding/json"
	"flag"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// listOpts are the shared --status, --sort and --limit flags of the list verbs.
type listOpts struct {
	status string
	sort   string
	limit  int
}

func registerListOpts(fs *flag.FlagSet) *listOpts {
	o := &listOpts{}
	fs.StringVar(&o.status, "status", "", "only rows whose status matches (case-insensitive)")
	fs.StringVar(&o.sort, "sort", "", "sort by a field; prefix with - for descending (name, updated, created or any JSON field)")
	fs.IntVar(&o.limit, "limit", 0, "show at most this many rows after filtering and sorting; 0 means all")
	return o
}

// query is the canonical query string for servers that filter, sort and cap themselves.
func (o *listOpts) query() string {
	v := url.Values{}
	if o.status != "" {
		v.Set("status", o.status)
	}
	if o.sort != "" {
		v.Set("sort", o.sort)
	}
	if o.limit > 0 {
		v.Set("limit", strconv.Itoa(o.limit))
	}
	return v.Encode()
}

func sortField(key string) string {
	switch key {
	case "updated":
		return "updated_at"
	case "created":
		return "created_at"
	}
	return key
}

func statusOf(m map[string]any) string {
	switch s := m["status"].(type) {
	case string:
		return s
	case map[string]any:
		for _, k := range []string{"label", "variant", "state"} {
			if v, ok := s[k].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

// applyListOpts filters, sorts and caps items client-side, in that order, so a
// limit never cuts rows before the sort sees them.
func applyListOpts[T any](items []T, o *listOpts) []T {
	if o == nil || (o.status == "" && o.sort == "" && o.limit <= 0) {
		return items
	}
	type row struct {
		item T
		m    map[string]any
	}
	rows := make([]row, 0, len(items))
	for _, it := range items {
		b, err := json.Marshal(it)
		if err != nil {
			continue
		}
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		if o.status != "" && !strings.EqualFold(statusOf(m), o.status) {
			continue
		}
		rows = append(rows, row{it, m})
	}
	if o.sort != "" {
		desc := strings.HasPrefix(o.sort, "-")
		key := sortField(strings.TrimPrefix(o.sort, "-"))
		sort.SliceStable(rows, func(i, j int) bool {
			a, b := toSortString(rows[i].m[key]), toSortString(rows[j].m[key])
			if desc {
				return a > b
			}
			return a < b
		})
	}
	if o.limit > 0 && len(rows) > o.limit {
		rows = rows[:o.limit]
	}
	out := make([]T, len(rows))
	for i, r := range rows {
		out[i] = r.item
	}
	return out
}

func toSortString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.ToLower(t)
	case float64:
		return strconv.FormatFloat(t, 'f', 6, 64)
	case nil:
		return ""
	}
	b, _ := json.Marshal(v)
	return string(b)
}
