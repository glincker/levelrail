package k8simport

import (
	"sort"
	"strings"
)

// Severity ranks how much an unmapped field matters.
type Severity string

// Finding severities. A blocker stops a non-dry-run import unless forced.
const (
	SeverityIgnoredSafe     Severity = "ignored-safe"
	SeverityChangesBehavior Severity = "changes-behavior"
	SeverityBlocker         Severity = "blocker"
)

// Finding is one field the importer could not map faithfully. It never
// carries a Secret or ConfigMap value, only names and paths.
type Finding struct {
	Kind     string   `json:"kind"`
	Name     string   `json:"name"`
	Field    string   `json:"field"`
	Reason   string   `json:"reason"`
	Severity Severity `json:"severity"`
}

// AppSummary describes one app the import would create.
type AppSummary struct {
	Name       string   `json:"name"`
	Source     string   `json:"source"`
	Image      string   `json:"image"`
	Port       int      `json:"port,omitempty"`
	Replicas   int      `json:"replicas"`
	Domains    []string `json:"domains,omitempty"`
	EnvKeys    []string `json:"envKeys,omitempty"`
	SecretKeys []string `json:"secretKeys,omitempty"`
}

// Result is the outcome of a conversion. Documents hold no secret value.
type Result struct {
	// Documents is multi-document YAML in the stack format apply accepts.
	Documents string       `json:"documents"`
	Apps      []AppSummary `json:"apps"`
	Findings  []Finding    `json:"findings"`
	secrets   map[string]string
}

// Secrets returns "app/NAME" to value for the apply step. Callers must
// never print or log it.
func (r *Result) Secrets() map[string]string {
	out := make(map[string]string, len(r.secrets))
	for k, v := range r.secrets {
		out[k] = v
	}
	return out
}

// Blockers counts blocker findings.
func (r *Result) Blockers() int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == SeverityBlocker {
			n++
		}
	}
	return n
}

func (r *Result) add(sev Severity, kind, name, field, reason string) {
	r.Findings = append(r.Findings, Finding{Kind: kind, Name: name, Field: field, Reason: reason, Severity: sev})
}

func (r *Result) sortFindings() {
	rank := map[Severity]int{SeverityBlocker: 0, SeverityChangesBehavior: 1, SeverityIgnoredSafe: 2}
	seen := map[Finding]bool{}
	out := r.Findings[:0]
	for _, f := range r.Findings {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	r.Findings = out
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if rank[a.Severity] != rank[b.Severity] {
			return rank[a.Severity] < rank[b.Severity]
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Field != b.Field {
			return a.Field < b.Field
		}
		return strings.Compare(a.Reason, b.Reason) < 0
	})
}

// Namespace handling modes for Options.NamespaceAs.
const (
	NamespaceEnvironment = "environment"
	NamespacePrefix      = "prefix"
	NamespaceIgnore      = "ignore"
)

// Options tune a conversion.
type Options struct {
	AppPrefix   string
	NamespaceAs string
	// Project names the project documents when NamespaceAs is environment.
	Project string
	Limits  Limits
}
