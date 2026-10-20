package telemetry

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// LogQL label names the node-local store can answer for.
const (
	LogQLLabelApp    = "app"
	LogQLLabelStream = "stream"
	LogQLLabelLevel  = "level"
)

const maxLogQLLen = 4096

// LogQLMatcher is one {label op "value"} selector term.
type LogQLMatcher struct {
	Name  string
	Op    string
	Value string
	re    *regexp.Regexp
}

// Matches reports whether the label value satisfies the matcher.
func (m LogQLMatcher) Matches(value string) bool {
	switch m.Op {
	case "=":
		return value == m.Value
	case "!=":
		return value != m.Value
	case "=~":
		return m.re.MatchString(value)
	default:
		return !m.re.MatchString(value)
	}
}

// LogQLLineFilter is one |= != |~ !~ line filter.
type LogQLLineFilter struct {
	Op    string
	Value string
	re    *regexp.Regexp
}

// Matches reports whether the line passes the filter.
func (f LogQLLineFilter) Matches(line string) bool {
	switch f.Op {
	case "|=":
		return strings.Contains(line, f.Value)
	case "!=":
		return !strings.Contains(line, f.Value)
	case "|~":
		return f.re.MatchString(line)
	default:
		return !f.re.MatchString(line)
	}
}

// LogQLQuery is a parsed stream selector plus line filters.
type LogQLQuery struct {
	Matchers []LogQLMatcher
	Filters  []LogQLLineFilter
}

// MatchLabels reports whether every selector term holds for the label set.
func (q LogQLQuery) MatchLabels(labels map[string]string) bool {
	for _, m := range q.Matchers {
		if !m.Matches(labels[m.Name]) {
			return false
		}
	}
	return true
}

// MatchApp applies only the app terms, used to pick which apps to read.
func (q LogQLQuery) MatchApp(app string) bool {
	for _, m := range q.Matchers {
		if m.Name == LogQLLabelApp && !m.Matches(app) {
			return false
		}
	}
	return true
}

// MatchLine applies every line filter in order.
func (q LogQLQuery) MatchLine(line string) bool {
	for _, f := range q.Filters {
		if !f.Matches(line) {
			return false
		}
	}
	return true
}

var logQLMetricFuncs = map[string]bool{
	"rate": true, "count_over_time": true, "bytes_over_time": true, "bytes_rate": true,
	"sum": true, "avg": true, "min": true, "max": true, "count": true, "topk": true,
	"bottomk": true, "sort": true, "sort_desc": true, "absent_over_time": true,
	"vector": true, "label_replace": true, "stddev": true, "stdvar": true,
}

// ParseLogQL parses the supported LogQL subset: a stream selector on app,
// stream and level followed by line filters. Everything else is an error
// rather than a quietly wrong answer.
func ParseLogQL(src string) (LogQLQuery, error) {
	s := strings.TrimSpace(src)
	if s == "" {
		return LogQLQuery{}, errors.New("empty query: expected a stream selector such as {app=\"web\"}")
	}
	if len(s) > maxLogQLLen {
		return LogQLQuery{}, errors.New("query is too long")
	}
	if s[0] != '{' {
		if i := strings.IndexAny(s, "( "); i > 0 && logQLMetricFuncs[s[:i]] {
			return LogQLQuery{}, fmt.Errorf("metric queries are not supported (%s); only log queries are", s[:i])
		}
		return LogQLQuery{}, errors.New("query must start with a stream selector such as {app=\"web\"}")
	}
	p := &logQLParser{s: s, pos: 1}
	var q LogQLQuery
	if err := p.parseSelector(&q); err != nil {
		return LogQLQuery{}, err
	}
	if err := p.parseFilters(&q); err != nil {
		return LogQLQuery{}, err
	}
	hasApp := false
	for _, m := range q.Matchers {
		if m.Name == LogQLLabelApp && (m.Op == "=" || m.Op == "=~") {
			hasApp = true
		}
	}
	if !hasApp {
		return LogQLQuery{}, errors.New("selector needs an app matcher with = or =~, for example {app=\"web\"}")
	}
	return q, nil
}

type logQLParser struct {
	s   string
	pos int
}

func (p *logQLParser) skipSpace() {
	for p.pos < len(p.s) && (p.s[p.pos] == ' ' || p.s[p.pos] == '\t' || p.s[p.pos] == '\n') {
		p.pos++
	}
}

func (p *logQLParser) parseSelector(q *LogQLQuery) error {
	for {
		p.skipSpace()
		if p.pos >= len(p.s) {
			return errors.New("unterminated stream selector, expected }")
		}
		if p.s[p.pos] == '}' {
			p.pos++
			break
		}
		start := p.pos
		for p.pos < len(p.s) && isLogQLIdent(p.s[p.pos]) {
			p.pos++
		}
		name := p.s[start:p.pos]
		if name == "" {
			return fmt.Errorf("unexpected %q in stream selector at position %d", string(p.s[p.pos]), p.pos)
		}
		if name != LogQLLabelApp && name != LogQLLabelStream && name != LogQLLabelLevel {
			return fmt.Errorf("unsupported label %q: only app, stream and level can be selected", name)
		}
		p.skipSpace()
		op := p.readOp("=~", "!~", "!=", "=")
		if op == "" {
			return fmt.Errorf("expected =, !=, =~ or !~ after label %q", name)
		}
		p.skipSpace()
		val, err := p.readString()
		if err != nil {
			return err
		}
		m := LogQLMatcher{Name: name, Op: op, Value: val}
		if op == "=~" || op == "!~" {
			if m.re, err = regexp.Compile("^(?:" + val + ")$"); err != nil {
				return fmt.Errorf("invalid regex for label %q: %w", name, err)
			}
		}
		q.Matchers = append(q.Matchers, m)
		p.skipSpace()
		if p.pos < len(p.s) && p.s[p.pos] == ',' {
			p.pos++
		}
	}
	if len(q.Matchers) == 0 {
		return errors.New("empty stream selector: add an app matcher such as {app=\"web\"}")
	}
	return nil
}

func (p *logQLParser) parseFilters(q *LogQLQuery) error {
	for {
		p.skipSpace()
		if p.pos >= len(p.s) {
			return nil
		}
		rest := p.s[p.pos:]
		if strings.HasPrefix(rest, "|") && !strings.HasPrefix(rest, "|=") && !strings.HasPrefix(rest, "|~") {
			return errors.New("parser and label stages (json, logfmt, pattern, line_format, unwrap) are not supported; only line filters |= != |~ !~ are")
		}
		op := p.readOp("|=", "|~", "!=", "!~")
		if op == "" {
			return fmt.Errorf("unexpected %q after the selector; only line filters |= != |~ !~ are supported", string(rest[0]))
		}
		p.skipSpace()
		val, err := p.readString()
		if err != nil {
			return err
		}
		f := LogQLLineFilter{Op: op, Value: val}
		if op == "|~" || op == "!~" {
			if f.re, err = regexp.Compile(val); err != nil {
				return fmt.Errorf("invalid line filter regex: %w", err)
			}
		}
		q.Filters = append(q.Filters, f)
	}
}

func (p *logQLParser) readOp(ops ...string) string {
	for _, op := range ops {
		if strings.HasPrefix(p.s[p.pos:], op) {
			p.pos += len(op)
			return op
		}
	}
	return ""
}

func (p *logQLParser) readString() (string, error) {
	if p.pos >= len(p.s) {
		return "", errors.New("expected a quoted string")
	}
	quote := p.s[p.pos]
	if quote != '"' && quote != '`' {
		return "", fmt.Errorf("expected a quoted string at position %d", p.pos)
	}
	i := p.pos + 1
	for i < len(p.s) {
		if quote == '"' && p.s[i] == '\\' {
			i += 2
			continue
		}
		if p.s[i] == quote {
			raw := p.s[p.pos : i+1]
			p.pos = i + 1
			val, err := strconv.Unquote(raw)
			if err != nil {
				return "", fmt.Errorf("invalid string literal %s", raw)
			}
			return val, nil
		}
		i++
	}
	return "", errors.New("unterminated string literal")
}

func isLogQLIdent(c byte) bool {
	return c == '_' || c == '.' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
