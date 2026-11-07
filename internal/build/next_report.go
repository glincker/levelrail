package build

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Route kinds in a NextReport.
const (
	NextKindStatic  = "static"
	NextKindISR     = "isr"
	NextKindDynamic = "dynamic"
	NextKindUnknown = "unknown"
)

const (
	nextReportMaxWarnings = 50
	nextReportMaxWarnLen  = 300
)

// NextRoute is one row of the Next.js build route table. Sizes are kB (1000 bytes
// as Next prints them) and read 0 when the build log has no size columns.
type NextRoute struct {
	Path        string  `json:"path"`
	Kind        string  `json:"kind"`
	SizeKB      float64 `json:"size_kb"`
	FirstLoadKB float64 `json:"first_load_kb"`
}

// NextReport is the structured form of a Next.js build log.
type NextReport struct {
	Detected        bool        `json:"detected"`
	Routes          []NextRoute `json:"routes"`
	SharedFirstLoad *float64    `json:"shared_first_load_kb"`
	Warnings        []string    `json:"warnings"`
}

var (
	nextANSI       = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	nextBuildxPre  = regexp.MustCompile(`^#\d+\s+\d+(?:\.\d+)?\s`)
	nextHeader     = regexp.MustCompile(`^Route \((?:app|pages)\)`)
	nextRow        = regexp.MustCompile(`^[┌├└─]\s+([^\s\w/\[┌├└│─])\s+(/\S*)\s*(.*)$`)
	nextSize       = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(B|kB|KB|MB)\b`)
	nextRevalidate = regexp.MustCompile(`(?:^|\s)\d+[smhdwy](?:\s|$)`)
	nextShared     = regexp.MustCompile(`First Load JS shared by all\s+(\d+(?:\.\d+)?)\s*(B|kB|KB|MB)`)
	nextLegend     = regexp.MustCompile(`^([^\s\w])\s+\(([^)]+)\)`)
	nextBanner     = regexp.MustCompile(`Next\.js \d+\.\d+`)
	nextDetectedLn = regexp.MustCompile(`^next\.js \d+ detected\b`)
	nextWarn       = regexp.MustCompile(`(?i)^(?:⚠\s*|warn(?:ing)?[:\s]\s*)|\bWarning:`)
)

// ParseNextReport reads a Next.js build log. Unknown formats and missing columns
// are tolerated: it never fails, it just reports less.
func ParseNextReport(lines []string) NextReport {
	rep := NextReport{Routes: []NextRoute{}, Warnings: []string{}}
	var raw []nextRawRow
	legend := map[string]string{}
	inTable := false
	seenWarn := map[string]bool{}
	for _, l := range lines {
		for _, ln := range strings.Split(l, "\n") {
			ln = cleanNextLine(ln)
			if nextBanner.MatchString(ln) {
				rep.Detected = true
			}
			if nextHeader.MatchString(ln) {
				rep.Detected, inTable = true, true
				continue
			}
			if m := nextShared.FindStringSubmatch(ln); m != nil {
				v := nextKB(m[1], m[2])
				rep.SharedFirstLoad = &v
				continue
			}
			if m := nextLegend.FindStringSubmatch(ln); m != nil && legendKind(m[2]) != "" {
				legend[m[1]] = legendKind(m[2])
				continue
			}
			if ln == "" {
				inTable = false
				continue
			}
			if w, ok := nextWarning(ln); ok && !seenWarn[w] && len(rep.Warnings) < nextReportMaxWarnings {
				seenWarn[w] = true
				rep.Warnings = append(rep.Warnings, w)
			}
			if !inTable {
				continue
			}
			if m := nextRow.FindStringSubmatch(ln); m != nil {
				raw = append(raw, nextRouteOf(m[1], m[2], m[3]))
			}
		}
	}
	for _, row := range raw {
		r := row.NextRoute
		if k, ok := legend[row.sym]; ok && k != NextKindUnknown {
			r.Kind = k
		} else if k := defaultNextKind(row.sym); k != "" {
			r.Kind = k
		} else {
			r.Kind = NextKindUnknown
		}
		// A revalidate time on a prerendered row is what makes it ISR.
		if row.revalidate && r.Kind == NextKindStatic {
			r.Kind = NextKindISR
		}
		rep.Routes = append(rep.Routes, r)
	}
	return rep
}

// nextRawRow keeps the row symbol until the legend, printed after the table, is known.
type nextRawRow struct {
	NextRoute
	sym        string
	revalidate bool
}

func cleanNextLine(s string) string {
	s = nextANSI.ReplaceAllString(s, "")
	s = strings.TrimRight(s, " \t\r")
	s = nextBuildxPre.ReplaceAllString(s, "")
	return strings.TrimLeft(s, " \t")
}

func nextRouteOf(sym, path, tail string) nextRawRow {
	r := NextRoute{Path: path}
	sizes := nextSize.FindAllStringSubmatch(tail, 2)
	if len(sizes) > 0 {
		r.SizeKB = nextKB(sizes[0][1], sizes[0][2])
	}
	if len(sizes) > 1 {
		r.FirstLoadKB = nextKB(sizes[1][1], sizes[1][2])
	}
	rest := nextSize.ReplaceAllString(tail, " ")
	return nextRawRow{NextRoute: r, sym: sym, revalidate: nextRevalidate.MatchString(" " + rest + " ")}
}

func nextKB(num, unit string) float64 {
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0
	}
	switch unit {
	case "B":
		v /= 1000
	case "MB":
		v *= 1000
	}
	return math.Round(v*100) / 100
}

func legendKind(desc string) string {
	d := strings.ToLower(desc)
	switch {
	case strings.Contains(d, "isr") || strings.Contains(d, "incremental"):
		return NextKindISR
	case strings.Contains(d, "partial"):
		return NextKindUnknown
	case strings.Contains(d, "static") || strings.Contains(d, "ssg"):
		return NextKindStatic
	case strings.Contains(d, "dynamic") || strings.Contains(d, "server"):
		return NextKindDynamic
	}
	return ""
}

func defaultNextKind(sym string) string {
	switch sym {
	case "○", "●", "✓":
		return NextKindStatic
	case "ƒ", "λ":
		return NextKindDynamic
	}
	return ""
}

func nextWarning(ln string) (string, bool) {
	if !nextWarn.MatchString(ln) {
		return "", false
	}
	w := strings.TrimSpace(strings.TrimPrefix(ln, "⚠"))
	if len(w) > nextReportMaxWarnLen {
		w = w[:nextReportMaxWarnLen]
	}
	return w, w != ""
}

// IsNextDetectedLine reports whether a build log line is the platform's own
// "next.js N detected" note, so callers can record the framework server side.
func IsNextDetectedLine(line string) bool {
	return nextDetectedLn.MatchString(strings.ToLower(cleanNextLine(line)))
}
