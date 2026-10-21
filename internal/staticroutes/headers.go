package staticroutes

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// ParseHeaders parses Netlify _headers syntax: a path line, then indented
// "Name: value" lines, or "! Name" to remove a header.
func ParseHeaders(src string, l Limits) ([]HeaderBlock, []Problem) {
	var blocks []HeaderBlock
	var probs []Problem
	var cur *HeaderBlock
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
			probs = append(probs, Problem{Source: SourceHeaders, Line: lineNo, Severity: SeverityError, Message: fmt.Sprintf(format, a...)})
		}
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if raw[0] != ' ' && raw[0] != '\t' {
			flush()
			skip = false
			b := HeaderBlock{Source: fmt.Sprintf("%s:%d", SourceHeaders, lineNo), Path: trimmed}
			pat, err := validateHeaderPath(trimmed, l)
			if err != nil {
				bad("%s", err)
				skip = true
				continue
			}
			b.pat = pat
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
		if err := addHeaderLine(cur, trimmed, l); err != nil {
			bad("%s", err)
		}
	}
	flush()
	return blocks, probs
}

func validateHeaderPath(p string, l Limits) (*pattern, error) {
	if len(p) > l.MaxValueLen || !trafficpolicy.SafeHeaderValue(p) || strings.ContainsAny(p, " \t") {
		return nil, fmt.Errorf("path %q must be a single path without spaces", p)
	}
	return compilePattern(p)
}

func addHeaderLine(b *HeaderBlock, line string, l Limits) error {
	if strings.HasPrefix(line, "!") {
		name := strings.TrimSpace(line[1:])
		if err := checkHeaderName(name); err != nil {
			return err
		}
		b.Remove = append(b.Remove, http.CanonicalHeaderKey(name))
		return nil
	}
	name, value, ok := strings.Cut(line, ":")
	name, value = strings.TrimSpace(name), strings.TrimSpace(value)
	if !ok || value == "" {
		return fmt.Errorf("%q must be Name: value", line)
	}
	if err := checkHeaderName(name); err != nil {
		return err
	}
	if len(value) > l.MaxValueLen || !trafficpolicy.SafeHeaderValue(value) {
		return fmt.Errorf("value of %s must be a single line of at most %d bytes", name, l.MaxValueLen)
	}
	b.Set = append(b.Set, HeaderKV{Name: http.CanonicalHeaderKey(name), Value: value})
	return nil
}

func checkHeaderName(name string) error {
	if name == "" {
		return fmt.Errorf("header name is empty")
	}
	if msg := trafficpolicy.ResponseHeaderNameProblem(name); msg != "" {
		return fmt.Errorf("header %q: %s", name, msg)
	}
	return nil
}

// ValidateHeaderBlock checks a block built from app.yaml and prepares it.
func ValidateHeaderBlock(b *HeaderBlock, l Limits) error {
	pat, err := validateHeaderPath(b.Path, l)
	if err != nil {
		return err
	}
	scratch := HeaderBlock{}
	for _, kv := range b.Set {
		if err := addHeaderLine(&scratch, kv.Name+": "+kv.Value, l); err != nil {
			return err
		}
	}
	for _, name := range b.Remove {
		if err := addHeaderLine(&scratch, "! "+name, l); err != nil {
			return err
		}
	}
	b.Set, b.Remove, b.pat = scratch.Set, scratch.Remove, pat
	return nil
}
