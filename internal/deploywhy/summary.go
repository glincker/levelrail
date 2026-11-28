package deploywhy

import (
	"fmt"
	"sort"
	"strings"
)

// Cause is the classified failure with its fix text.
type Cause struct {
	Code string `json:"code"`
	Text string `json:"cause"`
	Fix  string `json:"fix"`
}

// Change is one before and after pair for a scalar setting.
type Change struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Changes is what differs from the last good deploy. Env holds key names only, never values.
type Changes struct {
	Image      *Change  `json:"image,omitempty"`
	Port       *Change  `json:"port,omitempty"`
	EnvAdded   []string `json:"env_added,omitempty"`
	EnvRemoved []string `json:"env_removed,omitempty"`
	EnvChanged []string `json:"env_changed,omitempty"`
}

// Empty reports whether nothing differs.
func (c Changes) Empty() bool {
	return c.Image == nil && c.Port == nil && len(c.EnvAdded)+len(c.EnvRemoved)+len(c.EnvChanged) == 0
}

// TailLine is one log line with the number of consecutive repeats folded into it.
type TailLine struct {
	Text  string `json:"text"`
	Count int    `json:"count"`
}

// CollapseTail folds consecutive repeats of lines (already secret-masked) and keeps the last n entries.
func CollapseTail(lines []string, n int) []TailLine {
	out := []TailLine{}
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if k := len(out) - 1; k >= 0 && out[k].Text == l {
			out[k].Count++
			continue
		}
		out = append(out, TailLine{Text: l, Count: 1})
	}
	if n > 0 && len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

// Summarize renders the five-line explanation: cause, fix, exit and health, log tail, changes.
func Summarize(r Report) []string {
	cause, fix := "cause unknown", "read the log lines and retry the deploy"
	if r.Cause != nil {
		cause = fmt.Sprintf("%s (%s)", r.Cause.Text, r.Cause.Code)
		fix = r.Cause.Fix
	}
	signals := []string{}
	if r.ExitCode != nil {
		s := fmt.Sprintf("exit code %d", *r.ExitCode)
		if r.OOMKilled {
			s += ", oom killed"
		}
		signals = append(signals, s)
	}
	if r.Health != nil {
		signals = append(signals, fmt.Sprintf("last health %s ok=%t status=%d", r.Health.Target, r.Health.Success, r.Health.StatusCode))
	}
	if len(signals) == 0 {
		signals = append(signals, "no exit or health record")
	}
	tail := "no log lines captured"
	if len(r.Tail) > 0 {
		parts := make([]string, 0, len(r.Tail))
		for _, t := range r.Tail {
			if t.Count > 1 {
				parts = append(parts, fmt.Sprintf("%s (x%d)", t.Text, t.Count))
			} else {
				parts = append(parts, t.Text)
			}
		}
		tail = strings.Join(parts, " | ")
	}
	return []string{
		"Cause: " + cause,
		"Fix: " + fix,
		"Signals: " + strings.Join(signals, "; "),
		"Last lines: " + tail,
		"Changed: " + describeChanges(r.Changes, r.BaselineID),
	}
}

func describeChanges(c Changes, baseline string) string {
	if baseline == "" {
		return "no earlier good deploy to compare"
	}
	if c.Empty() {
		return "nothing since " + baseline
	}
	var parts []string
	if c.Image != nil {
		parts = append(parts, fmt.Sprintf("image %s to %s", c.Image.From, c.Image.To))
	}
	if c.Port != nil {
		parts = append(parts, fmt.Sprintf("port %s to %s", c.Port.From, c.Port.To))
	}
	for _, g := range []struct {
		label string
		keys  []string
	}{{"env added", c.EnvAdded}, {"env removed", c.EnvRemoved}, {"env changed", c.EnvChanged}} {
		if len(g.keys) > 0 {
			k := append([]string(nil), g.keys...)
			sort.Strings(k)
			parts = append(parts, g.label+" "+strings.Join(k, ","))
		}
	}
	return strings.Join(parts, "; ") + " since " + baseline
}
