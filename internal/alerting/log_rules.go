package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Log rule kinds. Both are app-scoped and evaluated from the node-local log
// store on a bounded cadence, never over an unbounded range.
const (
	// KindLogPattern fires when lines matching a text, regex or field filter
	// reach Count within Window.
	KindLogPattern Kind = "log_pattern"
	// KindLogLevelRate fires when lines at or above Level arrive faster than
	// PerMinute over Window.
	KindLogLevelRate Kind = "log_level_rate"
)

// Env vars and defaults bounding log rule evaluation.
const (
	envLogRuleEvalEvery = "APP_LOG_RULE_EVAL_INTERVAL"
	envLogRuleMaxWindow = "APP_LOG_RULE_MAX_WINDOW"
	envLogRuleMaxRows   = "APP_LOG_RULE_MAX_ROWS"
	envLogRuleSamples   = "APP_LOG_RULE_SAMPLES"

	DefaultLogRuleEvalEvery = time.Minute
	DefaultLogRuleMaxWindow = time.Hour
	DefaultLogRuleMaxRows   = 5000
	DefaultLogRuleSamples   = 5

	defaultLogRuleWindow = 5 * time.Minute
	maxLogRegexLen       = 200
	maxLogSampleLen      = 300
)

// LogRulePolicy bounds one evaluation: how often a rule runs, how far back
// it may look and how many rows it may read.
type LogRulePolicy struct {
	EvalEvery time.Duration
	MaxWindow time.Duration
	MaxRows   int
	Samples   int
}

// LogRulePolicyFromEnv applies APP_LOG_RULE_* overrides to the defaults. A
// malformed or non-positive value keeps the default.
func LogRulePolicyFromEnv() LogRulePolicy {
	p := LogRulePolicy{EvalEvery: DefaultLogRuleEvalEvery, MaxWindow: DefaultLogRuleMaxWindow, MaxRows: DefaultLogRuleMaxRows, Samples: DefaultLogRuleSamples}
	dur := func(key string, dst *time.Duration) {
		if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
			*dst = d
		}
	}
	num := func(key string, dst *int) {
		if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
			*dst = n
		}
	}
	dur(envLogRuleEvalEvery, &p.EvalEvery)
	dur(envLogRuleMaxWindow, &p.MaxWindow)
	num(envLogRuleMaxRows, &p.MaxRows)
	num(envLogRuleSamples, &p.Samples)
	return p
}

// LogRuleConfig is what a log_pattern or log_level_rate rule watches.
type LogRuleConfig struct {
	// Query is a full-text phrase matched by the store's FTS index.
	Query string `json:"query,omitempty"`
	// Regex further narrows the lines by their message (RE2 syntax).
	Regex string `json:"regex,omitempty"`
	// Fields are key=value, key!=value, key~text or key>number filters.
	Fields []string `json:"fields,omitempty"`
	// Level keeps lines at or above it. log_level_rate defaults to error.
	Level string `json:"level,omitempty"`
	// Count is the match count that fires a log_pattern rule.
	Count int `json:"count,omitempty"`
	// PerMinute is the lines per minute that fires a log_level_rate rule.
	PerMinute float64 `json:"per_minute,omitempty"`
	// WindowSeconds is the look-back; zero means five minutes.
	WindowSeconds int `json:"window_seconds,omitempty"`
	// CooldownSeconds suppresses a new firing for this long after the last one.
	CooldownSeconds int `json:"cooldown_seconds,omitempty"`
}

// Window returns the look-back with its default applied.
func (c LogRuleConfig) Window() time.Duration {
	if c.WindowSeconds <= 0 {
		return defaultLogRuleWindow
	}
	return time.Duration(c.WindowSeconds) * time.Second
}

// Validate rejects a config that cannot be evaluated within policy.
func (c LogRuleConfig) Validate(kind Kind, p LogRulePolicy) error {
	if c.Window() > p.MaxWindow {
		return fmt.Errorf("window_seconds must be at most %d", int(p.MaxWindow/time.Second))
	}
	if c.WindowSeconds < 0 || c.CooldownSeconds < 0 {
		return errors.New("window_seconds and cooldown_seconds must not be negative")
	}
	if c.Level != "" && telemetry.NormalizeLevel(c.Level) == "" {
		return errors.New("level must be one of trace, debug, info, warn, error, fatal")
	}
	if len(c.Regex) > maxLogRegexLen {
		return fmt.Errorf("regex must be at most %d characters", maxLogRegexLen)
	}
	if c.Regex != "" {
		if _, err := regexp.Compile(c.Regex); err != nil {
			return fmt.Errorf("regex: %w", err)
		}
	}
	if _, err := telemetry.ParseFieldFilters(c.Fields); err != nil {
		return err
	}
	switch kind {
	case KindLogPattern:
		if c.Query == "" && c.Regex == "" && len(c.Fields) == 0 && c.Level == "" {
			return errors.New("a log_pattern rule needs a query, regex, field filter or level")
		}
		if c.Count < 1 {
			return errors.New("count must be at least 1")
		}
		if c.Count > p.MaxRows {
			return fmt.Errorf("count must be at most %d (the rows read per evaluation)", p.MaxRows)
		}
	case KindLogLevelRate:
		if c.PerMinute <= 0 {
			return errors.New("per_minute must be greater than 0")
		}
	default:
		return fmt.Errorf("kind %q is not a log rule", kind)
	}
	return nil
}

func encodeLogRule(c *LogRuleConfig) string {
	if c == nil {
		return ""
	}
	b, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return string(b)
}

func decodeLogRule(raw string) *LogRuleConfig {
	if raw == "" {
		return nil
	}
	var c LogRuleConfig
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil
	}
	return &c
}

// LogRuleResult is one evaluation's outcome.
type LogRuleResult struct {
	Matched   int
	Value     float64
	Truncated bool
	Samples   []string
	Notice    string
	Satisfied bool
}

// LogRuleSource reads bounded log windows; *telemetry.Federator satisfies it.
type LogRuleSource interface {
	QueryLogs(ctx context.Context, resourceID string, from, to time.Time, query string) ([]telemetry.LogEntry, error)
}

// MatchLogRule applies cfg to entries (oldest first, already limited to the
// window) and returns the result. It is pure so rules test with seeded lines.
func MatchLogRule(kind Kind, cfg LogRuleConfig, entries []telemetry.LogEntry, truncated bool, now time.Time, policy LogRulePolicy) (LogRuleResult, error) {
	var re *regexp.Regexp
	if cfg.Regex != "" {
		var err error
		if re, err = regexp.Compile(cfg.Regex); err != nil {
			return LogRuleResult{}, fmt.Errorf("compile regex: %w", err)
		}
	}
	filters, err := telemetry.ParseFieldFilters(cfg.Fields)
	if err != nil {
		return LogRuleResult{}, err
	}
	level := cfg.Level
	if kind == KindLogLevelRate && level == "" {
		level = telemetry.LevelError
	}
	minRank := telemetry.LevelRank(telemetry.NormalizeLevel(level))

	var matched []telemetry.LogEntry
	for _, e := range entries {
		if minRank > 0 && telemetry.LevelRank(telemetry.ClassifyLevel(e)) < minRank {
			continue
		}
		if re != nil && !re.MatchString(e.Message) {
			continue
		}
		if !telemetry.MatchFieldFilters(e, filters) {
			continue
		}
		matched = append(matched, e)
	}

	res := LogRuleResult{Matched: len(matched), Truncated: truncated}
	window := cfg.Window()
	switch kind {
	case KindLogPattern:
		res.Value = float64(res.Matched)
		res.Satisfied = res.Matched >= cfg.Count
		res.Notice = fmt.Sprintf("%d matching lines in the last %s (fires at %d)", res.Matched, window, cfg.Count)
	case KindLogLevelRate:
		span := window
		if truncated && len(entries) > 0 {
			span = max(now.Sub(entries[0].Timestamp), time.Second)
		}
		res.Value = float64(res.Matched) / span.Minutes()
		res.Satisfied = res.Value >= cfg.PerMinute
		res.Notice = fmt.Sprintf("%.1f %s lines per minute over %s (fires at %.1f)", res.Value, level, span.Round(time.Second), cfg.PerMinute)
	default:
		return LogRuleResult{}, fmt.Errorf("kind %q is not a log rule", kind)
	}
	if truncated {
		res.Notice += fmt.Sprintf("; only the newest %d lines were read", len(entries))
	}
	for i := len(matched) - 1; i >= 0 && len(res.Samples) < policy.Samples; i-- {
		line := telemetry.RedactLine(strings.TrimSpace(matched[i].Message))
		if len(line) > maxLogSampleLen {
			line = line[:maxLogSampleLen]
		}
		res.Samples = append(res.Samples, line)
	}
	for i, j := 0, len(res.Samples)-1; i < j; i, j = i+1, j-1 {
		res.Samples[i], res.Samples[j] = res.Samples[j], res.Samples[i]
	}
	return res, nil
}

// EvaluateLogRule runs one log_pattern or log_level_rate rule: it reads at
// most policy.MaxRows of the newest lines in the window, matches them and
// advances the rule's state.
func EvaluateLogRule(ctx context.Context, src LogRuleSource, r Rule, policy LogRulePolicy, now time.Time) (Rule, LogRuleResult, error) {
	if r.LogRule == nil {
		return r, LogRuleResult{}, fmt.Errorf("alerting: evaluate rule %q: no log rule config", r.ID)
	}
	cfg := *r.LogRule
	window := min(cfg.Window(), policy.MaxWindow)
	cfg.WindowSeconds = int(window / time.Second)

	entries, truncated, err := telemetry.QueryLogsBounded(ctx, src, r.ResourceID, now.Add(-window), now, cfg.Query, policy.MaxRows)
	if err != nil && len(entries) == 0 {
		return r, LogRuleResult{}, fmt.Errorf("alerting: evaluate rule %q: read logs: %w", r.ID, err)
	}
	res, err := MatchLogRule(r.Kind, cfg, entries, truncated, now, policy)
	if err != nil {
		return r, LogRuleResult{}, fmt.Errorf("alerting: evaluate rule %q: %w", r.ID, err)
	}
	next := r
	next.LastEvaluatedAt = &now
	v := res.Value
	next.LastValue = &v
	return advanceState(next, r, res.Satisfied, r.ForDuration, now), res, nil
}
