package tui

import "strings"

const (
	colGap      = 2
	colMaxWidth = 44
	colMinWidth = 4
)

// Cell is one table cell; Tone colours its text.
type Cell struct {
	Text string
	Tone Tone
}

// Row is one table row; Key identifies the resource and Ref names the
// app a drill-down should open (empty when there is none).
type Row struct {
	Key   string
	Ref   string
	Cells []Cell
}

func plain(s string) Cell { return Cell{Text: clean(s), Tone: toneNone} }

func toned(s string, t Tone) Cell { return Cell{Text: clean(s), Tone: t} }

// columnWidths fits natural widths into total, shrinking the widest first.
func columnWidths(cols []string, rows []Row, total int) []int {
	w := make([]int, len(cols))
	for i, c := range cols {
		w[i] = width(c)
	}
	for _, r := range rows {
		for i, c := range r.Cells {
			if i < len(w) {
				if cw := width(c.Text); cw > w[i] {
					w[i] = cw
				}
			}
		}
	}
	for i := range w {
		if w[i] > colMaxWidth {
			w[i] = colMaxWidth
		}
	}
	for sum(w)+colGap*(len(w)-1) > total {
		widest := 0
		for i := range w {
			if w[i] > w[widest] {
				widest = i
			}
		}
		if w[widest] <= colMinWidth {
			break
		}
		w[widest]--
	}
	return w
}

func sum(xs []int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}

// renderTable draws a header plus the visible window of rows. Lines are
// padded to totalW so the selected row's reverse video spans the screen.
func renderTable(st styles, cols []string, rows []Row, totalW, visible, cursor, offset int) []string {
	w := columnWidths(cols, rows, totalW)
	head := make([]string, len(cols))
	for i, c := range cols {
		head[i] = truncate(c, w[i])
		if i < len(cols)-1 {
			head[i] = pad(head[i], w[i])
		}
	}
	lines := []string{st.muted.Render(strings.Join(head, strings.Repeat(" ", colGap)))}
	for i := offset; i < len(rows) && i < offset+visible; i++ {
		lines = append(lines, renderRow(st, rows[i], w, totalW, i == cursor))
	}
	return lines
}

func renderRow(st styles, r Row, w []int, totalW int, selected bool) string {
	gap := strings.Repeat(" ", colGap)
	parts := make([]string, len(w))
	for i := range w {
		var c Cell
		if i < len(r.Cells) {
			c = r.Cells[i]
		}
		text := truncate(c.Text, w[i])
		if i < len(w)-1 {
			text = pad(text, w[i])
		}
		if !selected {
			text = st.tone[c.Tone].Render(text)
		}
		parts[i] = text
	}
	line := strings.Join(parts, gap)
	if selected {
		return st.selected.Render(pad(truncate(stripCells(r, w, gap), totalW), totalW))
	}
	return line
}

// stripCells rebuilds a row from plain text for reverse-video rendering.
func stripCells(r Row, w []int, gap string) string {
	parts := make([]string, len(w))
	for i := range w {
		text := ""
		if i < len(r.Cells) {
			text = r.Cells[i].Text
		}
		parts[i] = pad(truncate(text, w[i]), w[i])
	}
	return strings.Join(parts, gap)
}
