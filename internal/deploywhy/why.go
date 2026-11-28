// Package deploywhy turns the stored facts of one deploy into a short verdict.
package deploywhy

import "sort"

// Verdict names the strongest signal found, in priority order.
const (
	VerdictOOMKilled    = "oom_killed"
	VerdictExitNonZero  = "exit_nonzero"
	VerdictHealthFailed = "health_failing"
	VerdictNewLogs      = "new_log_patterns"
	VerdictNoSignal     = "no_signal"
	VerdictOK           = "ok"
)

// Template is one log cluster; Sample is already secret-masked.
type Template struct {
	TemplateID string `json:"template_id"`
	Template   string `json:"template"`
	Count      int    `json:"count"`
	Sample     string `json:"sample"`
}

// Health is the last probe result recorded for the deploy.
type Health struct {
	Target     string `json:"target"`
	Success    bool   `json:"success"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Input is everything Compute needs, all read from stored records.
type Input struct {
	DeployID         string
	Failed           bool
	ExitCode         *int
	OOMKilled        bool
	Health           *Health
	Templates        []Template
	BaselineID       string
	BaselineTemplate []Template
	Cause            *Cause
	TailLines        []string
	TailMax          int
	Changes          Changes
}

// Report is the verdict and its evidence.
type Report struct {
	DeployID         string     `json:"deploy_id"`
	Verdict          string     `json:"verdict"`
	ExitCode         *int       `json:"exit_code,omitempty"`
	OOMKilled        bool       `json:"oom_killed,omitempty"`
	Health           *Health    `json:"health,omitempty"`
	BaselineID       string     `json:"baseline_deploy_id,omitempty"`
	NewTemplateCount int        `json:"new_template_count"`
	NewTemplates     []Template `json:"new_templates"`
	Cause            *Cause     `json:"cause,omitempty"`
	Tail             []TailLine `json:"tail,omitempty"`
	Changes          Changes    `json:"changes"`
	Summary          []string   `json:"summary"`
}

// Compute picks the verdict. A template is new when the last good deploy has no cluster with the same id.
func Compute(in Input) Report {
	known := make(map[string]bool, len(in.BaselineTemplate))
	for _, t := range in.BaselineTemplate {
		known[t.TemplateID] = true
	}
	fresh := []Template{}
	for _, t := range in.Templates {
		if !known[t.TemplateID] {
			fresh = append(fresh, t)
		}
	}
	sort.SliceStable(fresh, func(i, j int) bool { return fresh[i].Count > fresh[j].Count })
	r := Report{DeployID: in.DeployID, ExitCode: in.ExitCode, OOMKilled: in.OOMKilled, Health: in.Health,
		BaselineID: in.BaselineID, NewTemplateCount: len(fresh), NewTemplates: fresh,
		Cause: in.Cause, Tail: CollapseTail(in.TailLines, in.TailMax), Changes: in.Changes}
	switch {
	case in.OOMKilled:
		r.Verdict = VerdictOOMKilled
	case in.ExitCode != nil && *in.ExitCode != 0:
		r.Verdict = VerdictExitNonZero
	case in.Health != nil && !in.Health.Success:
		r.Verdict = VerdictHealthFailed
	case len(fresh) > 0 && in.Failed:
		r.Verdict = VerdictNewLogs
	case in.Failed:
		r.Verdict = VerdictNoSignal
	default:
		r.Verdict = VerdictOK
	}
	r.Summary = Summarize(r)
	return r
}
