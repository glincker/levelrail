// Package importreport is the one fidelity report shape shared by the compose
// and docker run importers: every dropped or changed field with a plain reason.
package importreport

import "sort"

// Severity ranks how much a finding matters; names match the k8s import.
type Severity string

// Finding severities. A blocker means the import cannot deploy as written.
const (
	SeverityIgnoredSafe     Severity = "ignored-safe"
	SeverityChangesBehavior Severity = "changes-behavior"
	SeverityBlocker         Severity = "blocker"
)

// Source kinds a finding can come from.
const (
	KindCompose   = "compose"
	KindDockerRun = "docker-run"
)

// Finding is one field that was dropped, changed or needs attention. It never
// carries a value, only names and paths.
type Finding struct {
	Kind     string   `json:"kind"`
	Path     string   `json:"path"`
	Reason   string   `json:"reason"`
	Severity Severity `json:"severity"`
	Fix      string   `json:"fix,omitempty"`
}

func rank(s Severity) int {
	switch s {
	case SeverityBlocker:
		return 0
	case SeverityChangesBehavior:
		return 1
	}
	return 2
}

// Normalize drops duplicate paths (keeping the most severe) and sorts the
// result by severity, then kind and path, so output is stable for goldens.
func Normalize(in []Finding) []Finding {
	best := map[string]Finding{}
	for _, f := range in {
		k := f.Kind + "\x00" + f.Path
		if cur, ok := best[k]; !ok || rank(f.Severity) < rank(cur.Severity) {
			best[k] = f
		}
	}
	out := make([]Finding, 0, len(best))
	for _, f := range best {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if rank(a.Severity) != rank(b.Severity) {
			return rank(a.Severity) < rank(b.Severity)
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Path < b.Path
	})
	return out
}

// Blockers counts blocker findings.
func Blockers(in []Finding) int {
	n := 0
	for _, f := range in {
		if f.Severity == SeverityBlocker {
			n++
		}
	}
	return n
}
