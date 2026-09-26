package changes

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Notification size limits: a message carries at most NotifyMaxLines changes,
// each clipped to NotifyLineCap runes.
const (
	NotifyMaxLines = 5
	NotifyLineCap  = 120
)

// Line renders one change as a single short line, without a timestamp.
func (c Change) Line() string {
	var b strings.Builder
	b.WriteString(c.Title)
	var extra []string
	if len(c.Keys) > 0 && !strings.Contains(c.Title, c.Keys[0]) {
		extra = append(extra, strings.Join(c.Keys, ", "))
	}
	if c.Detail != "" {
		extra = append(extra, c.Detail)
	}
	if c.Actor != "" {
		extra = append(extra, "by "+c.Actor)
	}
	if len(extra) > 0 {
		b.WriteString(" (" + strings.Join(extra, "; ") + ")")
	}
	if c.LikelyCause {
		b.WriteString(" [likely cause]")
	}
	return clip(b.String(), NotifyLineCap)
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-3]) + "..."
}

// NotifyLines renders the top NotifyMaxLines changes, newest first, plus an
// "and N more" line. It returns nil when there is nothing to say.
func NotifyLines(r Result) []string {
	if len(r.Changes) == 0 {
		return nil
	}
	out := make([]string, 0, NotifyMaxLines+1)
	for i, c := range r.Changes {
		if i == NotifyMaxLines {
			break
		}
		out = append(out, c.At.UTC().Format("15:04")+" "+c.Line())
	}
	if more := r.Total - len(out); more > 0 {
		out = append(out, fmt.Sprintf("and %d more", more))
	}
	return out
}

// NotifyBlock renders the "Recent changes" section of a notification, ending
// with link when it is non-empty. It returns "" when nothing changed.
func NotifyBlock(r Result, link string) string {
	lines := NotifyLines(r)
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Changed in the last %s:\n- %s", humanWindow(r.Window), strings.Join(lines, "\n- "))
	if link != "" {
		b.WriteString("\n" + link)
	}
	return b.String()
}

func humanWindow(d time.Duration) string {
	switch {
	case d >= time.Hour && d%time.Hour == 0:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	case d >= time.Minute && d%time.Minute == 0:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	default:
		return d.Round(time.Second).String()
	}
}
