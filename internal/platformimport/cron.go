package platformimport

import (
	"regexp"
	"strings"
)

var jobNameRe = regexp.MustCompile(`[^a-z0-9_-]+`)

// SanitizeJobName turns a source task name into a valid scheduled job name.
func SanitizeJobName(s string) string {
	s = strings.Trim(jobNameRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-"), "-_")
	if s == "" {
		return "task"
	}
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}

// NormalizeSchedule expands the named frequencies a source accepts into a cron expression.
func NormalizeSchedule(s string) string {
	s = strings.TrimSpace(s)
	switch strings.ToLower(strings.TrimPrefix(s, "@")) {
	case "hourly":
		return "0 * * * *"
	case "daily", "midnight":
		return "0 0 * * *"
	case "weekly":
		return "0 0 * * 0"
	case "monthly":
		return "0 0 1 * *"
	case "yearly", "annually":
		return "0 0 1 1 *"
	}
	return s
}
