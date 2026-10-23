package spec

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Static routing sources, severities and sidecar naming.
const (
	StaticSourceRedirects = "_redirects"
	StaticSourceHeaders   = "_headers"
	StaticSeverityError   = "error"
	StaticSeverityWarning = "warning"
	// StaticSidecarSuffix names the compiled rules file stored beside a published tree.
	StaticSidecarSuffix = ".routes.json"
)

// StaticProblem is one validation finding. Line is 0 for spec rules.
type StaticProblem struct {
	Source   string `json:"source"`
	Line     int    `json:"line,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// StaticRuleset is everything compiled for one deploy.
type StaticRuleset struct {
	Version  int                 `json:"version"`
	Rules    []StaticRule        `json:"rules"`
	Headers  []StaticHeaderBlock `json:"headers"`
	Options  StaticServeOptions  `json:"options"`
	Problems []StaticProblem     `json:"problems"`
}

var statusTokenRe = regexp.MustCompile(`^([0-9]{3})(!?)$`)

func isStaticQueryToken(tok string) bool {
	return strings.Contains(tok, "=") && !strings.HasPrefix(tok, "/") && !strings.HasPrefix(tok, "http://") && !strings.HasPrefix(tok, "https://")
}

// ParseStaticRedirects parses Netlify _redirects syntax: "from [key=val ...] to
// [status[!]]" per line. Bad lines are skipped and reported, never applied
// half understood.
func ParseStaticRedirects(src string, l StaticLimits) ([]StaticRule, []StaticProblem) {
	var rules []StaticRule
	var probs []StaticProblem
	for i, line := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lineNo := i + 1
		bad := func(format string, a ...any) {
			probs = append(probs, StaticProblem{Source: StaticSourceRedirects, Line: lineNo, Severity: StaticSeverityError, Message: fmt.Sprintf(format, a...)})
		}
		r, err := parseStaticRedirectLine(line)
		if err != nil {
			bad("%s", err)
			continue
		}
		r.Source = fmt.Sprintf("%s:%d", StaticSourceRedirects, lineNo)
		if err := ValidateStaticRule(&r, l); err != nil {
			bad("%s", err)
			continue
		}
		rules = append(rules, r)
	}
	return rules, probs
}

func parseStaticRedirectLine(line string) (StaticRule, error) {
	f := strings.Fields(line)
	r := StaticRule{From: f[0], Status: 301}
	if !strings.HasPrefix(r.From, "/") {
		return r, fmt.Errorf("from %q must be a path starting with /; host based rules are not supported", r.From)
	}
	rest := f[1:]
	for len(rest) > 0 && isStaticQueryToken(rest[0]) {
		k, v, _ := strings.Cut(rest[0], "=")
		r.Query = append(r.Query, StaticQueryMatch{Key: k, Value: v})
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return r, fmt.Errorf("missing the destination after %q", r.From)
	}
	r.To, rest = rest[0], rest[1:]
	for i, tok := range rest {
		if m := statusTokenRe.FindStringSubmatch(tok); m != nil && i == 0 {
			r.Status, _ = strconv.Atoi(m[1])
			r.Force = m[2] == "!"
			continue
		}
		if strings.Contains(tok, "=") {
			return r, fmt.Errorf("condition %q is not supported (country, language, role and cookie conditions need an edge runtime)", tok)
		}
		return r, fmt.Errorf("unexpected %q, expected a status code such as 301, with a ! suffix to force it", tok)
	}
	return r, nil
}

// ParseStaticHeaders parses Netlify _headers syntax: a path line, then indented
// "Name: value" lines, or "! Name" to remove a header.
func ParseStaticHeaders(src string, l StaticLimits) ([]StaticHeaderBlock, []StaticProblem) {
	var blocks []StaticHeaderBlock
	var probs []StaticProblem
	var cur *StaticHeaderBlock
	skip := false
	flush := func() {
		if cur != nil {
			blocks = append(blocks, *cur)
			cur = nil
		}
	}
	for i, raw := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		lineNo := i + 1
		bad := func(format string, a ...any) {
			probs = append(probs, StaticProblem{Source: StaticSourceHeaders, Line: lineNo, Severity: StaticSeverityError, Message: fmt.Sprintf(format, a...)})
		}
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if raw[0] != ' ' && raw[0] != '\t' {
			flush()
			skip = false
			b := StaticHeaderBlock{Source: fmt.Sprintf("%s:%d", StaticSourceHeaders, lineNo), Path: trimmed}
			pat, err := ValidateStaticHeaderPath(trimmed, l)
			if err != nil {
				bad("%s", err)
				skip = true
				continue
			}
			b.SetPattern(pat)
			cur = &b
			continue
		}
		if skip {
			continue
		}
		if cur == nil {
			bad("header line %q has no path above it", trimmed)
			continue
		}
		if err := AddStaticHeaderLine(cur, trimmed, l); err != nil {
			bad("%s", err)
		}
	}
	flush()
	return blocks, probs
}

// staticRulesetVersion is bumped when the sidecar shape changes incompatibly.
const staticRulesetVersion = 1

// maxStaticSidecarBytes bounds what the ingress will read back for one site.
const maxStaticSidecarBytes = 8 << 20

// CompileStatic validates every source and returns the ruleset. Invalid file lines
// become problems and are skipped; an invalid app.yaml entry, a bad option or
// a cap breach is an error, so the deploy fails before anything is served.
func CompileStatic(in StaticInput, l StaticLimits) (*StaticRuleset, error) {
	rs := &StaticRuleset{Version: staticRulesetVersion, Options: in.Options, Rules: []StaticRule{}, Headers: []StaticHeaderBlock{}, Problems: []StaticProblem{}}
	rules, headers, err := CompileStaticEntries(in.Routes, in.HeaderBlocks, &rs.Options, l)
	if err != nil {
		return nil, err
	}
	rs.Rules, rs.Headers = rules, headers
	for _, f := range []struct {
		name, text string
	}{{StaticSourceRedirects, in.Redirects}, {StaticSourceHeaders, in.Headers}} {
		if len(f.text) > l.MaxFileBytes {
			return nil, fmt.Errorf("%s is %d bytes, over the %d byte cap (%s)", f.name, len(f.text), l.MaxFileBytes, EnvStaticMaxFileBytes)
		}
	}
	rules, probs := ParseStaticRedirects(in.Redirects, l)
	rs.Rules = append(rs.Rules, rules...)
	blocks, hprobs := ParseStaticHeaders(in.Headers, l)
	rs.Headers = append(rs.Headers, blocks...)
	rs.Problems = append(rs.Problems, probs...)
	rs.Problems = append(rs.Problems, hprobs...)
	if len(rs.Rules) > l.MaxRules {
		return nil, fmt.Errorf("%d routing rules is over the cap of %d (%s)", len(rs.Rules), l.MaxRules, EnvStaticMaxRules)
	}
	if len(rs.Headers) > l.MaxHeaderBlocks {
		return nil, fmt.Errorf("%d header blocks is over the cap of %d (%s)", len(rs.Headers), l.MaxHeaderBlocks, EnvStaticMaxHeaderBlocks)
	}
	return rs, nil
}

// Prepare compiles the matchers of a ruleset read back from JSON.
func (rs *StaticRuleset) Prepare() error {
	l := StaticLimits{MaxRules: 1 << 20, MaxHeaderBlocks: 1 << 20, MaxFileBytes: maxStaticSidecarBytes, MaxValueLen: 1 << 16}
	for i := range rs.Rules {
		if err := ValidateStaticRule(&rs.Rules[i], l); err != nil {
			return fmt.Errorf("staticroutes: rule %s: %w", rs.Rules[i].Source, err)
		}
	}
	for i := range rs.Headers {
		if err := ValidateStaticHeaderBlock(&rs.Headers[i], l); err != nil {
			return fmt.Errorf("staticroutes: headers %s: %w", rs.Headers[i].Source, err)
		}
	}
	return ValidateStaticOptions(&rs.Options)
}

// StaticSidecarPath is where the ruleset of the tree at rootDir is stored: beside
// the tree, so it is never served and follows the tree on rollback.
func StaticSidecarPath(rootDir string) string {
	return filepath.Clean(rootDir) + StaticSidecarSuffix
}

// WriteStaticSidecar stores rs beside the tree at rootDir, replacing any earlier one.
func WriteStaticSidecar(rootDir string, rs *StaticRuleset) error {
	raw, err := json.Marshal(rs)
	if err != nil {
		return fmt.Errorf("staticroutes: encode rules: %w", err)
	}
	dest := StaticSidecarPath(rootDir)
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".routes-*.tmp")
	if err != nil {
		return fmt.Errorf("staticroutes: write rules: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("staticroutes: write rules: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("staticroutes: write rules: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o640); err != nil { //nolint:gosec // owner and group read only
		return fmt.Errorf("staticroutes: write rules: %w", err)
	}
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return fmt.Errorf("staticroutes: write rules: %w", err)
	}
	return nil
}

// LoadStaticSidecar reads the ruleset of the tree at rootDir. A tree published
// before routing rules existed has none and yields nil without an error.
func LoadStaticSidecar(rootDir string) (*StaticRuleset, error) {
	f, err := os.Open(StaticSidecarPath(rootDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("staticroutes: open rules: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only handle
	var rs StaticRuleset
	dec := json.NewDecoder(io.LimitReader(f, maxStaticSidecarBytes))
	if err := dec.Decode(&rs); err != nil {
		return nil, fmt.Errorf("staticroutes: decode rules: %w", err)
	}
	if rs.Version != staticRulesetVersion {
		return nil, fmt.Errorf("staticroutes: rules version %d is not supported", rs.Version)
	}
	if err := rs.Prepare(); err != nil {
		return nil, err
	}
	return &rs, nil
}
