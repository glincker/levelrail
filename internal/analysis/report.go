package analysis

import (
	"fmt"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/rightsizing"
	"github.com/GLINCKER/levelrail/kit/untrusted"
)

// Build turns a digest and its trigger into a report. It is pure: same
// input, same output, and every number is copied from the digest.
func Build(tr Trigger, d Digest, cfg Config, now time.Time) Report {
	tr.Detail = untrusted.Sanitize(tr.Detail, cfg.LogSampleBytes)
	tr.RuleName = untrusted.Sanitize(tr.RuleName, cfg.LogSampleBytes)
	r := Report{
		SchemaVersion: SchemaVersion, App: d.App, GeneratedAt: now.UTC(), Trigger: tr, Digest: d,
		Timeline: d.Timeline, Causes: []Cause{}, Recommendations: []Recommendation{},
	}
	if r.Timeline == nil {
		r.Timeline = []Event{}
	}
	if !hasData(d) {
		r.Status = StatusInsufficient
		r.Summary = fmt.Sprintf("Insufficient data for %s: no metrics, logs, deploys, restarts or alerts in the last %s.", d.App, rightsizing.HumanWindow(cfg.Window))
		return r
	}
	r.Status = StatusComplete
	r.Causes = rankCauses(tr, d, cfg)
	r.Recommendations = recommend(tr, d, r.Causes)
	r.Summary = summary(d, r.Causes, cfg)
	return r
}

func hasData(d Digest) bool {
	for _, m := range d.Metrics {
		if m.Samples > 0 {
			return true
		}
	}
	return len(d.Logs) > 0 || len(d.Timeline) > 0
}

func rankCauses(tr Trigger, d Digest, cfg Config) []Cause {
	var cs []Cause
	add := func(id, title string, score int, ev ...string) {
		cs = append(cs, Cause{ID: id, Title: title, Score: score, Confidence: confidence(score), Evidence: ev})
	}
	for i := len(d.Timeline) - 1; i >= 0; i-- {
		e := d.Timeline[i]
		if e.Kind != "deploy" || e.At.After(tr.At) || tr.At.Sub(e.At) > cfg.DeployProximity {
			continue
		}
		score := 60
		if len(e.Detail) >= 13 && e.Detail[:13] == "deploy failed" {
			score = 90
		}
		add("recent_deploy", fmt.Sprintf("Deploy at %s preceded the trigger", e.At.Format(time.RFC3339)), score, e.Ref)
		break
	}
	if d.Restarts >= cfg.RestartMin {
		ev := []string{}
		for _, e := range d.Timeline {
			if e.Kind == "restart" && len(ev) < 5 {
				ev = append(ev, e.Ref)
			}
		}
		add("restarts", fmt.Sprintf("%d container restarts in the window", d.Restarts), 80, ev...)
	}
	for _, m := range d.Metrics {
		if m.Samples == 0 {
			continue
		}
		limit := d.CPUPercentLimit
		if m.Metric == metricMemory {
			limit = float64(d.MemoryLimit)
		}
		switch {
		case limit > 0 && m.P95/limit*100 >= cfg.UtilizationPct:
			add(m.Metric+"_saturation", fmt.Sprintf("%s p95 is %.0f%% of its limit", m.Metric, m.P95/limit*100), 70, m.Evidence)
		case m.AnomalyCount > 0:
			add(m.Metric+"_anomaly", fmt.Sprintf("%s had %d anomalous samples (max z %.1f)", m.Metric, m.AnomalyCount, m.MaxZ), 55, m.Evidence)
		}
	}
	for _, g := range d.Logs {
		if (g.Level == "error" || g.Level == "fatal") && g.Count >= cfg.LogErrorMin {
			score := 45
			if g.Level == "fatal" {
				score = 50
			}
			add("log_errors", fmt.Sprintf("%d %s log lines share one pattern", g.Count, g.Level), score, g.Evidence)
			break
		}
	}
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].Score != cs[j].Score {
			return cs[i].Score > cs[j].Score
		}
		return cs[i].ID < cs[j].ID
	})
	for i := range cs {
		cs[i].Rank = i + 1
	}
	return cs
}

func confidence(score int) string {
	switch {
	case score >= 75:
		return "high"
	case score >= 50:
		return "medium"
	}
	return "low"
}

func recommend(tr Trigger, d Digest, causes []Cause) []Recommendation {
	recs := []Recommendation{}
	for _, c := range causes {
		var text string
		switch c.ID {
		case "recent_deploy":
			text = "Compare the latest deploy with the previous release and roll back if the trigger started right after it."
		case "restarts":
			text = "Inspect the app's exit reason and recent logs for the restarting container."
		case "log_errors":
			text = "Review the repeated error pattern in the logs and its first-seen time against the timeline."
		default:
			continue
		}
		recs = append(recs, Recommendation{ID: "rec_" + c.ID, Text: text, Evidence: c.Evidence})
	}
	for _, s := range d.Sizing {
		if !s.DataSufficient || s.Action == rightsizing.ActionKeep || s.Action == "" {
			continue
		}
		var cur string
		sug := s.SuggestedText
		if s.Dimension == "memory" {
			cur = rightsizing.FormatMemory(s.CurrentLimit)
		} else {
			cur = rightsizing.FormatCPU(s.CurrentLimit)
		}
		rec := Recommendation{
			ID:       "rec_size_" + s.Dimension,
			Text:     fmt.Sprintf("%s the %s limit from %s to %s (p95 usage %.0f, %s confidence).", capitalize(s.Action), s.Dimension, cur, sug, s.P95Usage, s.Confidence),
			Evidence: []string{"metric:" + map[string]string{"memory": metricMemory, "cpu": metricCPU}[s.Dimension]},
		}
		if s.Action == rightsizing.ActionLower && tr.Kind == TriggerSpend {
			rec.MonthlySavingUSD = d.MonthlySavingUSD
		}
		recs = append(recs, rec)
	}
	return recs
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-32) + s[1:]
}

func summary(d Digest, causes []Cause, cfg Config) string {
	head := "No dominant cause found"
	if len(causes) > 0 {
		head = fmt.Sprintf("Most likely: %s (%s confidence)", causes[0].Title, causes[0].Confidence)
	}
	return fmt.Sprintf("%s. %s: %d restarts, %d log lines, %d timeline events in the last %s.", head, d.App, d.Restarts, d.LogLines, len(d.Timeline), rightsizing.HumanWindow(cfg.Window))
}
