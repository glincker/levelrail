package api

import (
	"errors"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

// resolveUpdateWindow validates the window fields of a set request. Omitted
// fields leave the stored window alone, so the pair is only checked when given.
func resolveUpdateWindow(req setImageAutoUpdateRequest) (string, int, error) {
	if req.WindowCron == nil {
		return "", 0, nil
	}
	cron := strings.TrimSpace(*req.WindowCron)
	if cron == "" {
		return "", 0, nil
	}
	if req.WindowMinutes == nil || *req.WindowMinutes <= 0 {
		return "", 0, errors.New("window_minutes must be a positive number of minutes")
	}
	if err := cronexpr.Validate(cron, time.UTC, time.Now()); err != nil {
		return "", 0, err
	}
	return cron, *req.WindowMinutes, nil
}

// inUpdateWindow reports whether now falls inside a window that opens at a
// cron fire time and lasts minutes. An empty cron means always open.
func inUpdateWindow(cron string, minutes int, now time.Time) bool {
	if cron == "" || minutes <= 0 {
		return true
	}
	sched, err := cronexpr.Parse(cron)
	if err != nil {
		return true
	}
	start := cronexpr.NextInLocation(sched, now.Add(-time.Duration(minutes)*time.Minute), time.UTC)
	return !start.IsZero() && !start.After(now)
}
