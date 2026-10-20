package main

import (
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

const maxJobsNextCount = 50

// jobsNext prints the next count occurrences of a cron expression in a time
// zone, so a schedule can be checked before it is saved.
func jobsNext(schedule, timezone string, count int, from time.Time, stdout, stderr io.Writer) int {
	if schedule == "" {
		_, _ = fmt.Fprintln(stderr, "jobs next: --schedule is required")
		return exitUsage
	}
	sched, err := cronexpr.Parse(schedule)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "jobs next: %v\n", err)
		return exitUsage
	}
	if timezone == "" {
		timezone = "UTC"
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "jobs next: %q is not a known IANA time zone\n", timezone)
		return exitUsage
	}
	if count < 1 || count > maxJobsNextCount {
		count = 5
	}
	t := from
	for i := 0; i < count; i++ {
		t = cronexpr.NextInLocation(sched, t, loc)
		if t.IsZero() {
			break
		}
		_, _ = fmt.Fprintln(stdout, t.In(loc).Format(time.RFC3339))
	}
	return exitOK
}
