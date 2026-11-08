package main

import (
	"fmt"
	"io"
)

// noColorFlag is set by the global --no-color flag for the current run.
var noColorFlag bool

// extractNoColorFlag strips --no-color wherever it appears, like --debug.
func extractNoColorFlag(args []string) (remaining []string, set bool) {
	remaining = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			remaining = append(remaining, args[i:]...)
			break
		}
		if args[i] == "--no-color" {
			set = true
			continue
		}
		remaining = append(remaining, args[i])
	}
	return remaining, set
}

// termStyle paints text only when w is a terminal and color is not disabled, and falls back
// to ASCII symbols otherwise so piped output carries no escape bytes.
type termStyle struct{ color bool }

func styleFor(w io.Writer) termStyle {
	if noColorFlag || ciMode {
		return termStyle{}
	}
	if v, ok := rawLookupEnv(envNoColor); ok && v != "" {
		return termStyle{}
	}
	return termStyle{color: stdoutIsTerminal(w)}
}

const (
	ansiRed    = "31"
	ansiGreen  = "32"
	ansiYellow = "33"
)

func (s termStyle) paint(code, text string) string {
	if !s.color {
		return text
	}
	return fmt.Sprintf("\x1b[%sm%s\x1b[0m", code, text)
}

// symbol returns the glyph on a color terminal and the plain word otherwise.
func (s termStyle) symbol(glyph, ascii string) string {
	if s.color {
		return glyph
	}
	return ascii
}
