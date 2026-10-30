package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/untrusted"
)

// ModeSource returns the current AI control mode for the acting session.
type ModeSource func(ctx context.Context) string

// SetModeSource enables the write guard; nil leaves the server-side gate as the only check.
func (e *Engine) SetModeSource(m ModeSource) { e.modeFn = m }

// writeRefusal explains why a mutating call is refused in the current mode, or "" when allowed.
func (e *Engine) writeRefusal(ctx context.Context, t ToolTraits, name string) string {
	if e.modeFn == nil || (t.Known && t.ReadOnly) {
		return ""
	}
	mode := e.modeFn(ctx)
	if mode != store.AIControlModeOff && mode != store.AIControlModeObserve {
		return ""
	}
	return fmt.Sprintf("%s changes state and AI control mode %q does not allow changes. Do not retry: use read-only tools and tell the operator an administrator must raise the mode to operate.", name, mode)
}

// guardedTools wraps a ToolExecutor with per-turn loop, empty-result and error-scrub guards.
type guardedTools struct {
	ToolExecutor
	repeatLimit, emptyLimit int
	seen                    map[string]int
	empties                 int
}

func newGuardedTools(inner ToolExecutor, l Limits) *guardedTools {
	return &guardedTools{ToolExecutor: inner, repeatLimit: l.RepeatLimit, emptyLimit: l.EmptyLimit, seen: map[string]int{}}
}

func (g *guardedTools) Call(ctx context.Context, name string, args json.RawMessage) (string, bool, error) {
	key := name + "\x00" + string(args)
	g.seen[key]++
	if g.repeatLimit > 0 && g.seen[key] > g.repeatLimit {
		return fmt.Sprintf("%s was already called %d times with identical arguments. Stop calling it and answer with what you have.", name, g.repeatLimit), true, nil
	}
	text, isError, err := g.ToolExecutor.Call(ctx, name, args)
	if err != nil {
		return ScrubToolError(err.Error()), true, nil
	}
	if isError {
		return ScrubToolError(text), true, nil
	}
	if !isEmptyResult(text) {
		g.empties = 0
		return text, false, nil
	}
	g.empties++
	if g.emptyLimit > 0 && g.empties >= g.emptyLimit {
		text += "\nNote: several tool calls in a row returned nothing. Stop retrying and answer the operator with what you know."
	}
	return text, false, nil
}

func isEmptyResult(s string) bool {
	switch strings.TrimSpace(s) {
	case "", "[]", "{}", "null":
		return true
	}
	return false
}

var (
	stackStart = regexp.MustCompile(`(?m)^(goroutine \d+ \[|panic: |Traceback \(most recent call last\)|\s+at .+\(.+:\d+\)).*`)
	pathLike   = regexp.MustCompile(`(?:/(?:Users|home|root|var|opt|etc|usr|tmp|app|srv|data)/[^\s"'):,]+)|(?:[A-Za-z]:\\[^\s"']+)`)
)

// ScrubToolError removes secrets, internal paths and stack traces from a tool error before the model sees it.
func ScrubToolError(s string) string {
	if loc := stackStart.FindStringIndex(s); loc != nil {
		s = strings.TrimRight(s[:loc[0]], " \n\t") + "\n[stack trace removed]"
	}
	s = pathLike.ReplaceAllString(s, "[path]")
	return Scrub(untrusted.Redact(s), "")
}
