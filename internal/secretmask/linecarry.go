package secretmask

import "strings"

// Line is one log line with the stream it came from.
type Line struct {
	Stream string
	Text   string
}

// LineCarry masks a sequence of log lines while catching a secret that spans
// a line break. It holds back raw trailing lines covering the last maxLen-1
// bytes, so the caller must Flush after a short idle interval and at stream end.
type LineCarry struct {
	reg  *Registry
	pend []Line
}

// NewLineCarry masks with the process-wide registry.
func NewLineCarry() *LineCarry { return &LineCarry{reg: shared} }

// NewLineCarryFor masks with reg.
func NewLineCarryFor(reg *Registry) *LineCarry { return &LineCarry{reg: reg} }

// Pending reports whether raw lines are still held back.
func (c *LineCarry) Pending() bool { return len(c.pend) > 0 }

// Push adds a raw line and returns the masked lines that are now safe to emit.
func (c *LineCarry) Push(stream, text string) []Line {
	c.pend = append(c.pend, Line{Stream: stream, Text: text})
	return c.release(false)
}

// Flush emits everything held, masked.
func (c *LineCarry) Flush() []Line { return c.release(true) }

func (c *LineCarry) release(all bool) []Line {
	if len(c.pend) == 0 {
		return nil
	}
	texts := make([]string, len(c.pend))
	for i, l := range c.pend {
		texts[i] = l.Text
	}
	joined := strings.Join(texts, "\n")
	spans := c.reg.mergedSpans(joined)

	cut := len(joined)
	if !all {
		hold := c.reg.maxLen() - 1
		if hold < 0 {
			hold = 0
		}
		if hold > len(joined) {
			hold = len(joined)
		}
		cut = len(joined) - hold
		for _, sp := range spans {
			if sp[0] < cut && cut < sp[1] {
				cut = sp[0]
			}
		}
	}

	var out []Line
	start, n := 0, 0
	groupStart := -1
	for i, t := range texts {
		end := start + len(t)
		if !all && end > cut {
			break
		}
		if groupStart < 0 {
			groupStart = start
		}
		// A span over the newline after this line joins it to the next one.
		joins := false
		for _, sp := range spans {
			if sp[0] <= end && end < sp[1] {
				joins = true
				break
			}
		}
		if !joins || i == len(texts)-1 {
			out = append(out, Line{Stream: c.pend[n].Stream, Text: c.reg.Mask(joined[groupStart:end])})
			groupStart = -1
			n = i + 1
		}
		start = end + 1
	}
	c.pend = append([]Line(nil), c.pend[n:]...)
	return out
}
