package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// EnvLogMaxBytes names the env var that caps a compact log query's output.
	EnvLogMaxBytes = "APP_MCP_LOG_MAX_BYTES"
	// DefaultLogMaxBytes is the output cap when EnvLogMaxBytes is unset.
	DefaultLogMaxBytes = 8 << 10
	// DefaultLogMaxLines is the line cap when the caller sets none.
	DefaultLogMaxLines = 100
	// DefaultLogWindow is the lookback when neither since nor from is set.
	DefaultLogWindow = time.Hour
	// logLineMaxBytes clips one log line so a single huge line cannot eat the cap.
	logLineMaxBytes = 500
	// logEnvelopeReserve leaves room for the excerpt's own fields inside the byte cap.
	logEnvelopeReserve = 400
)

// LogQuery selects log lines of one app.
type LogQuery struct {
	App    string
	Deploy string
	Level  string
	Since  string
	Until  string
	Text   string
	// MaxLines caps returned lines; 0 means DefaultLogMaxLines.
	MaxLines int
}

// LogExcerpt is a capped, compact view of a log query result.
type LogExcerpt struct {
	App     string   `json:"app"`
	From    string   `json:"from"`
	To      string   `json:"to"`
	Matched int      `json:"matched"`
	Shown   int      `json:"shown"`
	Lines   []string `json:"lines"`
	Notice  string   `json:"notice,omitempty"`
}

// LogMaxBytesFromEnv returns the byte cap from EnvLogMaxBytes, falling
// back to DefaultLogMaxBytes on an unset or invalid value.
func LogMaxBytesFromEnv(lookup func(string) (string, bool)) int {
	if v, ok := lookup(EnvLogMaxBytes); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return n
		}
	}
	return DefaultLogMaxBytes
}

// ParseLogTime parses a log window bound: an RFC3339 timestamp, or a Go
// duration ("30m") meaning that long before now.
func ParseLogTime(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is neither an RFC3339 time nor a duration like 30m", s)
	}
	return now.Add(-d), nil
}

// DeployWindow returns the time range one deploy attempt owned: from its
// start to the start of the next newer attempt, or now if it is the newest.
func DeployWindow(attempts []DeployAttemptResource, id string, now time.Time) (from, to time.Time, err error) {
	sorted := slices.Clone(attempts)
	slices.SortFunc(sorted, func(a, b DeployAttemptResource) int { return a.StartedAt.Compare(b.StartedAt) })
	for i, a := range sorted {
		if a.ID != id {
			continue
		}
		to = now
		if i+1 < len(sorted) {
			to = sorted[i+1].StartedAt
		}
		return a.StartedAt, to, nil
	}
	return time.Time{}, time.Time{}, fmt.Errorf("deploy %q not found", id)
}

// QueryLogsCompact runs q against the logs API and returns a capped
// excerpt of the newest matching lines. The level, text and newest-N
// filtering happen server side; the byte cap is applied here.
func (c *Client) QueryLogsCompact(ctx context.Context, q LogQuery, maxBytes int, now time.Time) (LogExcerpt, error) {
	if q.App == "" {
		return LogExcerpt{}, fmt.Errorf("app is required")
	}
	to := now
	from := now.Add(-DefaultLogWindow)
	var err error
	if q.Deploy != "" {
		attempts, aerr := c.ListDeployAttempts(ctx, q.App)
		if aerr != nil {
			return LogExcerpt{}, fmt.Errorf("list deploy attempts for app %q: %w", q.App, aerr)
		}
		if from, to, err = DeployWindow(attempts, q.Deploy, now); err != nil {
			return LogExcerpt{}, fmt.Errorf("app %q: %w", q.App, err)
		}
	}
	if q.Since != "" {
		if from, err = ParseLogTime(q.Since, now); err != nil {
			return LogExcerpt{}, fmt.Errorf("since: %w", err)
		}
	}
	if q.Until != "" {
		if to, err = ParseLogTime(q.Until, now); err != nil {
			return LogExcerpt{}, fmt.Errorf("until: %w", err)
		}
	}
	if from.After(to) {
		return LogExcerpt{}, fmt.Errorf("since must not be after until")
	}
	maxLines := q.MaxLines
	if maxLines <= 0 {
		maxLines = DefaultLogMaxLines
	}
	if maxBytes <= 0 {
		maxBytes = DefaultLogMaxBytes
	}

	params := url.Values{}
	params.Set("from", from.UTC().Format(time.RFC3339))
	params.Set("to", to.UTC().Format(time.RFC3339))
	params.Set("limit", strconv.Itoa(maxLines))
	if q.Text != "" {
		params.Set("q", q.Text)
	}
	if q.Level != "" {
		params.Set("level", q.Level)
	}
	var out logsResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(q.App)+"/logs?"+params.Encode(), nil, &out); err != nil {
		return LogExcerpt{}, fmt.Errorf("query logs for app %q: %w", q.App, err)
	}
	total := max(out.Total, len(out.Entries))
	return BuildLogExcerpt(q.App, from, to, out.Entries, total, maxBytes), nil
}

// BuildLogExcerpt renders entries (oldest first) as one line each and
// drops the oldest lines until the output fits maxBytes. total is how
// many lines matched before any trimming.
func BuildLogExcerpt(app string, from, to time.Time, entries []LogEntryResource, total, maxBytes int) LogExcerpt {
	lines := make([]string, 0, len(entries))
	size := 0
	budget := maxBytes - min(logEnvelopeReserve, maxBytes/4)
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		line := e.Timestamp.UTC().Format("15:04:05")
		if e.Level != "" {
			line += " " + strings.ToUpper(e.Level)
		}
		line += " " + clipLine(e.Message, logLineMaxBytes)
		encoded, _ := json.Marshal(line)
		if size+len(encoded)+1 > budget {
			break
		}
		size += len(encoded) + 1
		lines = append(lines, line)
	}
	slices.Reverse(lines)
	ex := LogExcerpt{App: app, From: from.UTC().Format(time.RFC3339), To: to.UTC().Format(time.RFC3339), Matched: total, Shown: len(lines), Lines: lines}
	if len(lines) < total {
		ex.Notice = fmt.Sprintf("showing %s of %s matching lines (newest), use since/until or level to narrow", groupDigits(len(lines)), groupDigits(total))
	}
	return ex
}

func clipLine(s string, n int) string {
	s = strings.TrimRight(s, "\r\n")
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) && len(s) > 0 {
		s = s[:len(s)-1]
	}
	return s + "..."
}

func groupDigits(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
