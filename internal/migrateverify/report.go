// Package migrateverify proves a migrated or shadow-running instance matches
// the source platform before cutover. It compares names only: env var values
// never enter a snapshot, a report or a log line.
package migrateverify

import (
	"sort"
	"time"
)

// Severity ranks a difference. Only Blocking fails the verdict.
type Severity string

// Difference severities.
const (
	Blocking Severity = "blocking"
	Warning  Severity = "warning"
	Info     Severity = "info"
)

// Category groups differences by the resource they concern.
type Category string

// Difference categories.
const (
	CatApps      Category = "apps"
	CatDomains   Category = "domains"
	CatEnv       Category = "env"
	CatDatabases Category = "databases"
	CatVolumes   Category = "volumes"
	CatJobs      Category = "jobs"
	CatProbes    Category = "probes"
)

// Verdicts of a whole report.
const (
	VerdictPass = "pass"
	VerdictWarn = "warn"
	VerdictFail = "fail"
)

// Difference is one mismatch with a one-line fix hint.
type Difference struct {
	Category Category `json:"category"`
	Severity Severity `json:"severity"`
	App      string   `json:"app,omitempty"`
	Subject  string   `json:"subject"`
	Detail   string   `json:"detail"`
	Fix      string   `json:"fix"`
}

// Totals counts one category on each side.
type Totals struct {
	Category Category `json:"category"`
	Source   int      `json:"source"`
	Target   int      `json:"target"`
	Matched  int      `json:"matched"`
	Blocking int      `json:"blocking"`
	Warning  int      `json:"warning"`
	Info     int      `json:"info"`
}

// Counts is the number of differences per severity.
type Counts struct {
	Blocking int `json:"blocking"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
}

// Report is the full parity report. It holds names, counts and probe
// results only.
type Report struct {
	Platform    string        `json:"platform"`
	GeneratedAt time.Time     `json:"generated_at"`
	Verdict     string        `json:"verdict"`
	Counts      Counts        `json:"counts"`
	Categories  []Totals      `json:"categories"`
	Differences []Difference  `json:"differences"`
	Probes      []DomainProbe `json:"probes,omitempty"`
	Notes       []string      `json:"notes,omitempty"`
}

var severityRank = map[Severity]int{Blocking: 0, Warning: 1, Info: 2}

// Finish sorts the differences (blocking first), fills the counts and the verdict.
func (r *Report) Finish() {
	sort.SliceStable(r.Differences, func(i, j int) bool {
		a, b := r.Differences[i], r.Differences[j]
		if severityRank[a.Severity] != severityRank[b.Severity] {
			return severityRank[a.Severity] < severityRank[b.Severity]
		}
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		if a.App != b.App {
			return a.App < b.App
		}
		return a.Subject < b.Subject
	})
	r.Counts = Counts{}
	per := map[Category]*Totals{}
	for i := range r.Categories {
		t := &r.Categories[i]
		t.Blocking, t.Warning, t.Info = 0, 0, 0
		per[t.Category] = t
	}
	for _, d := range r.Differences {
		t := per[d.Category]
		switch d.Severity {
		case Blocking:
			r.Counts.Blocking++
			if t != nil {
				t.Blocking++
			}
		case Warning:
			r.Counts.Warning++
			if t != nil {
				t.Warning++
			}
		default:
			r.Counts.Info++
			if t != nil {
				t.Info++
			}
		}
	}
	switch {
	case r.Counts.Blocking > 0:
		r.Verdict = VerdictFail
	case r.Counts.Warning > 0:
		r.Verdict = VerdictWarn
	default:
		r.Verdict = VerdictPass
	}
}

// HasBlocking reports whether cutover should be held.
func (r Report) HasBlocking() bool { return r.Counts.Blocking > 0 }
