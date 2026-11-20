// Package pages holds the build log and dependency cache rules for git-connected Pages builds.
package pages

import "time"

// TruncatedNotice is the marker line shown when older output was dropped to fit the cap.
const TruncatedNotice = "[earlier build output truncated to fit the log size limit]"

// Window keeps the newest items whose summed size fits limit bytes. Failures show at the end of a
// build, so the tail is the part worth keeping. It reports whether anything was dropped.
func Window[T any](items []T, size func(T) int, limit int64) ([]T, bool) {
	if limit <= 0 {
		return items, false
	}
	var total int64
	start := len(items)
	for i := len(items) - 1; i >= 0; i-- {
		total += int64(size(items[i]) + 1)
		if total > limit {
			break
		}
		start = i
	}
	if start == 0 {
		return items, false
	}
	return items[start:], true
}

// LogExpired reports whether a finished deploy's log is past its retention. A zero retention
// keeps logs forever.
func LogExpired(finished, now time.Time, retention time.Duration) bool {
	if retention <= 0 || finished.IsZero() {
		return false
	}
	return now.Sub(finished) > retention
}
