package agentinit

import "strings"

// maxDiffCells bounds the LCS table so a huge existing file cannot force a
// quadratic allocation; past it Diff reports a whole-file replacement.
const maxDiffCells = 4 << 20

// Diff returns a line diff from old to next ("-" removed, "+" added, " "
// unchanged context omitted), empty when they are equal. It is a plain LCS
// diff, good enough for the small files init writes.
func Diff(old, next string) string {
	if old == next {
		return ""
	}
	a, b := splitLines(old), splitLines(next)
	if len(a) > maxDiffCells || len(b) > maxDiffCells || (len(a)+1)*(len(b)+1) > maxDiffCells {
		return replaceDiff(a, b)
	}
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out strings.Builder
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			i++
			j++
		case j < len(b) && (i == len(a) || lcs[i][j+1] >= lcs[i+1][j]):
			out.WriteString("+ " + b[j] + "\n")
			j++
		default:
			out.WriteString("- " + a[i] + "\n")
			i++
		}
	}
	return out.String()
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func replaceDiff(a, b []string) string {
	var out strings.Builder
	for _, l := range a {
		out.WriteString("- " + l + "\n")
	}
	for _, l := range b {
		out.WriteString("+ " + l + "\n")
	}
	return out.String()
}
