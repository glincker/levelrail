package api

import (
	"time"

	"github.com/glincker/levelrail/internal/telemetry"
)

// collapsedLog is one run of identical consecutive lines. The stored rows are
// never touched, so the raw lines stay reachable by repeating the query
// without collapse over [First, Last].
type collapsedLog struct {
	entry telemetry.LogEntry
	count int
	last  time.Time
}

// collapseLogs folds consecutive entries with the same stream, container and
// message into one, keeping the first entry as the representative.
func collapseLogs(entries []telemetry.LogEntry) []collapsedLog {
	out := make([]collapsedLog, 0, len(entries))
	for _, e := range entries {
		if n := len(out); n > 0 {
			prev := &out[n-1]
			if prev.entry.Stream == e.Stream && prev.entry.ContainerID == e.ContainerID && prev.entry.Message == e.Message {
				prev.count++
				prev.last = e.Timestamp
				continue
			}
		}
		out = append(out, collapsedLog{entry: e, count: 1, last: e.Timestamp})
	}
	return out
}

type collapseKey struct {
	ts                         time.Time
	stream, container, message string
}

// collapsedResources rebuilds resources for entries (possibly re-ranked after
// collapsing), attaching each run's count and last time. Runs sharing a key
// are handed out in order, so a repeated line never overwrites an earlier run.
func collapsedResources(entries []telemetry.LogEntry, runs []collapsedLog) []logEntryResource {
	byKey := make(map[collapseKey][]int, len(runs))
	for i, c := range runs {
		k := collapseKey{c.entry.Timestamp, c.entry.Stream, c.entry.ContainerID, c.entry.Message}
		byKey[k] = append(byKey[k], i)
	}
	out := make([]logEntryResource, len(entries))
	for i, e := range entries {
		r := toLogEntryResource(e)
		k := collapseKey{e.Timestamp, e.Stream, e.ContainerID, e.Message}
		if q := byKey[k]; len(q) > 0 {
			c := runs[q[0]]
			byKey[k] = q[1:]
			if c.count > 1 {
				r.Count = c.count
				r.First = e.Timestamp
				r.Last = c.last
			}
		}
		out[i] = r
	}
	return out
}
