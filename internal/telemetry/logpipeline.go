package telemetry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Pipeline rule kinds.
const (
	PipelineMultiline = "multiline"
	PipelineRedact    = "redact"
	PipelineParse     = "parse"
	PipelineDrop      = "drop"
)

const (
	maxPipelineRules   = 20
	maxPipelinePattern = 1024
	maxMultilineBytes  = 64 << 10
	pipelineCacheTTL   = 15 * time.Second
	// DefaultRedactReplacement replaces a redact match with no Replace set.
	DefaultRedactReplacement = "[REDACTED]"
)

// PipelineRule is one step. multiline: Pattern matches the first line of a
// record, other lines are joined to it. redact: matches are replaced.
// parse: named groups become structured fields. drop: matching lines go.
type PipelineRule struct {
	Kind    string `json:"kind"`
	Pattern string `json:"pattern"`
	Replace string `json:"replace,omitempty"`
}

// PreviewLine is the outcome of one sample line.
type PreviewLine struct {
	Message string            `json:"message"`
	Dropped bool              `json:"dropped"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// ValidatePipeline checks kinds and that every pattern compiles.
func ValidatePipeline(rules []PipelineRule) error {
	_, err := compilePipeline(rules)
	return err
}

type compiledRule struct {
	kind    string
	re      *regexp.Regexp
	replace string
}

type compiledPipeline struct {
	rules     []compiledRule
	multiline *regexp.Regexp
}

func compilePipeline(rules []PipelineRule) (*compiledPipeline, error) {
	if len(rules) > maxPipelineRules {
		return nil, fmt.Errorf("telemetry: a log pipeline holds at most %d rules", maxPipelineRules)
	}
	p := &compiledPipeline{}
	for i, r := range rules {
		switch r.Kind {
		case PipelineMultiline, PipelineRedact, PipelineParse, PipelineDrop:
		default:
			return nil, fmt.Errorf("telemetry: rule %d: unknown kind %q", i+1, r.Kind)
		}
		if r.Pattern == "" || len(r.Pattern) > maxPipelinePattern {
			return nil, fmt.Errorf("telemetry: rule %d: pattern must be 1 to %d characters", i+1, maxPipelinePattern)
		}
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, fmt.Errorf("telemetry: rule %d: invalid pattern: %w", i+1, err)
		}
		if r.Kind == PipelineParse && len(re.SubexpNames()) < 2 {
			return nil, fmt.Errorf("telemetry: rule %d: parse needs at least one named group", i+1)
		}
		if r.Kind == PipelineMultiline {
			if p.multiline != nil {
				return nil, fmt.Errorf("telemetry: rule %d: only one multiline rule is allowed", i+1)
			}
			p.multiline = re
			continue
		}
		rep := r.Replace
		if r.Kind == PipelineRedact && rep == "" {
			rep = DefaultRedactReplacement
		}
		p.rules = append(p.rules, compiledRule{kind: r.Kind, re: re, replace: rep})
	}
	return p, nil
}

// apply runs the per-line rules. It reports false when the line is dropped.
func (p *compiledPipeline) apply(msg string) (out string, fields map[string]string, keep bool) {
	if p == nil {
		return msg, nil, true
	}
	for _, r := range p.rules {
		switch r.kind {
		case PipelineDrop:
			if r.re.MatchString(msg) {
				return "", nil, false
			}
		case PipelineRedact:
			msg = r.re.ReplaceAllString(msg, r.replace)
		case PipelineParse:
			m := r.re.FindStringSubmatch(msg)
			if m == nil {
				continue
			}
			if fields == nil {
				fields = map[string]string{}
			}
			for i, n := range r.re.SubexpNames() {
				if n != "" && i < len(m) {
					fields[n] = m[i]
				}
			}
		}
	}
	return msg, fields, true
}

// PreviewPipeline runs rules over sample lines the way the collector does.
func PreviewPipeline(rules []PipelineRule, lines []string) ([]PreviewLine, error) {
	p, err := compilePipeline(rules)
	if err != nil {
		return nil, err
	}
	var records []string
	if p.multiline != nil {
		var cur []string
		for _, l := range lines {
			if p.multiline.MatchString(l) && len(cur) > 0 {
				records = append(records, strings.Join(cur, "\n"))
				cur = nil
			}
			cur = append(cur, l)
		}
		if len(cur) > 0 {
			records = append(records, strings.Join(cur, "\n"))
		}
	} else {
		records = lines
	}
	out := make([]PreviewLine, 0, len(records))
	for _, rec := range records {
		msg, fields, keep := p.apply(rec)
		out = append(out, PreviewLine{Message: msg, Dropped: !keep, Fields: fields})
	}
	return out, nil
}

// GetLogPipeline returns the rules for resourceID, nil when none are set.
func (db *DB) GetLogPipeline(ctx context.Context, resourceID string) ([]PipelineRule, error) {
	var raw string
	err := db.QueryRowContext(ctx, `SELECT rules_json FROM log_pipelines WHERE resource_id = ?`, resourceID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("telemetry: get log pipeline: %w", err)
	}
	var rules []PipelineRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil, fmt.Errorf("telemetry: decode log pipeline: %w", err)
	}
	return rules, nil
}

// SetLogPipeline validates and stores the rules; an empty list clears them.
func (db *DB) SetLogPipeline(ctx context.Context, resourceID string, rules []PipelineRule) error {
	if len(rules) == 0 {
		return db.DeleteLogPipeline(ctx, resourceID)
	}
	if err := ValidatePipeline(rules); err != nil {
		return err
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return fmt.Errorf("telemetry: encode log pipeline: %w", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO log_pipelines (resource_id, rules_json, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(resource_id) DO UPDATE SET rules_json = excluded.rules_json, updated_at = excluded.updated_at`,
		resourceID, string(raw), time.Now().Unix())
	if err != nil {
		return fmt.Errorf("telemetry: set log pipeline: %w", err)
	}
	return nil
}

// DeleteLogPipeline removes the rules for resourceID.
func (db *DB) DeleteLogPipeline(ctx context.Context, resourceID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM log_pipelines WHERE resource_id = ?`, resourceID); err != nil {
		return fmt.Errorf("telemetry: delete log pipeline: %w", err)
	}
	return nil
}

type pipelineCacheEntry struct {
	p       *compiledPipeline
	expires time.Time
}

// pipelineCache resolves a resource's compiled pipeline, reading the store
// at most once per TTL so a rule change applies within seconds.
type pipelineCache struct {
	mu sync.Mutex
	m  map[string]pipelineCacheEntry
}

func (c *pipelineCache) get(ctx context.Context, db *DB, resourceID string) *compiledPipeline {
	now := time.Now()
	c.mu.Lock()
	if e, ok := c.m[resourceID]; ok && now.Before(e.expires) {
		c.mu.Unlock()
		return e.p
	}
	c.mu.Unlock()
	var p *compiledPipeline
	if rules, err := db.GetLogPipeline(ctx, resourceID); err == nil && len(rules) > 0 {
		p, _ = compilePipeline(rules)
	}
	c.mu.Lock()
	if c.m == nil {
		c.m = map[string]pipelineCacheEntry{}
	}
	c.m[resourceID] = pipelineCacheEntry{p: p, expires: now.Add(pipelineCacheTTL)}
	c.mu.Unlock()
	return p
}
