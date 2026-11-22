package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"time"
)

// envWatchInterval seeds the default --interval of every --watch verb.
const envWatchInterval = "LEVELRAIL_WATCH_INTERVAL"

// watchOpts are the shared --watch and --interval flags of status and the list verbs.
type watchOpts struct {
	watch    bool
	interval time.Duration
}

// extraEnv names older per-verb variables that still win over the shared one.
func registerWatch(fs *flag.FlagSet, lookupEnv func(string) (string, bool), extraEnv ...string) *watchOpts {
	w := &watchOpts{}
	every := 5 * time.Second
	for _, name := range append(extraEnv, envWatchInterval) {
		if v, ok := lookupEnv(name); ok {
			if d, err := time.ParseDuration(v); err == nil && d > 0 {
				every = d
				break
			}
		}
	}
	fs.BoolVar(&w.watch, "watch", false, "redraw on an interval until Ctrl+C")
	fs.DurationVar(&w.interval, "interval", every, "redraw interval with --watch (env "+envWatchInterval+")")
	return w
}

// run draws once, or repeatedly with --watch; the screen is cleared only for human output.
func (w *watchOpts) run(stdout io.Writer, jsonOut bool, draw func() int) int {
	if !w.watch {
		return draw()
	}
	if w.interval <= 0 {
		w.interval = 5 * time.Second
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	tick := time.NewTicker(w.interval)
	defer tick.Stop()
	for {
		if !jsonOut {
			_, _ = io.WriteString(stdout, "\x1b[H\x1b[2J")
		}
		if code := draw(); code != exitOK {
			return code
		}
		select {
		case <-ctx.Done():
			return exitOK
		case <-tick.C:
		}
	}
}

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
