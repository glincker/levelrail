package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// styles holds every lipgloss style; with color off, only bold and
// reverse remain, so NO_COLOR output stays legible.
type styles struct {
	tone     map[Tone]lipgloss.Style
	bold     lipgloss.Style
	muted    lipgloss.Style
	selected lipgloss.Style
	tabOn    lipgloss.Style
	banner   lipgloss.Style
}

func newStyles(color bool) styles {
	s := styles{
		tone:     map[Tone]lipgloss.Style{},
		bold:     lipgloss.NewStyle().Bold(true),
		selected: lipgloss.NewStyle().Reverse(true),
		tabOn:    lipgloss.NewStyle().Bold(true).Underline(true),
		banner:   lipgloss.NewStyle().Bold(true),
	}
	for _, t := range []Tone{ToneNeutral, ToneSuccess, ToneWarning, ToneDanger, ToneInfo} {
		s.tone[t] = lipgloss.NewStyle()
	}
	s.muted = lipgloss.NewStyle()
	if color {
		s.tone[ToneSuccess] = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
		s.tone[ToneWarning] = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
		s.tone[ToneDanger] = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		s.tone[ToneInfo] = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
		s.tone[ToneNeutral] = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		s.muted = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		s.banner = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
	}
	return s
}

var terminalEscapes = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b.`)

// clean strips terminal escape sequences and control characters from
// server-supplied text so a log line or name cannot drive the terminal.
func clean(s string) string {
	s = terminalEscapes.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
			return -1
		}
		return r
	}, s)
}

func width(s string) int { return ansi.StringWidth(s) }

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

func pad(s string, w int) string {
	if n := w - width(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

func age(now, t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := now.Sub(t)
	switch {
	case d < 0:
		return "now"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func durationMS(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	d := time.Duration(ms) * time.Millisecond
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func shortID(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// logTone maps a log line to a level colour by its leading level token.
func logTone(line, stream string) Tone {
	u := strings.ToUpper(line)
	if len(u) > 160 {
		u = u[:160]
	}
	switch {
	case containsAny(u, "ERROR", "FATAL", "PANIC", "CRIT", " ERR "):
		return ToneDanger
	case containsAny(u, "WARN"):
		return ToneWarning
	case containsAny(u, "DEBUG", "TRACE"):
		return ToneNeutral
	case containsAny(u, "INFO"):
		return ToneInfo
	case stream == "stderr":
		return ToneWarning
	}
	return toneNone
}

// toneNone renders a log line with the terminal's default colour.
const toneNone Tone = -1

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
