package nodeinsight

import "time"

// Drain progress states, stable so the dashboard and CLI can translate them.
const (
	DrainRunning  = "running"
	DrainDone     = "done"
	DrainPartial  = "partial"
	DrainFailed   = "failed"
	DrainStalled  = "stalled"
	DrainNotFound = "none"
)

// DrainMove is one resource move inside a drain.
type DrainMove struct {
	Status     string
	StartedAt  string
	FinishedAt string
}

// DrainInput is what the store knows about a drain.
type DrainInput struct {
	Status    string
	Blocked   int
	Moves     []DrainMove
	StartedAt string
	UpdatedAt string
	// StallAfter marks a running drain with no update for this long as stalled.
	StallAfter time.Duration
}

// DrainProgress is the summary shown beside a drain.
type DrainProgress struct {
	State      string `json:"state"`
	Total      int    `json:"total"`
	Succeeded  int    `json:"succeeded"`
	Failed     int    `json:"failed"`
	RolledBack int    `json:"rolled_back"`
	Running    int    `json:"running"`
	Blocked    int    `json:"blocked"`
	Percent    int    `json:"percent"`
	// EtaSeconds is a rough estimate from finished moves, 0 when unknown.
	EtaSeconds int `json:"eta_seconds,omitempty"`
}

// BuildDrainProgress counts moves, estimates the time left from the mean
// duration of finished moves and flags a drain that stopped updating.
func BuildDrainProgress(in DrainInput, now time.Time) DrainProgress {
	p := DrainProgress{Blocked: in.Blocked}
	var spent time.Duration
	var timed int
	for _, m := range in.Moves {
		p.Total++
		switch m.Status {
		case "succeeded":
			p.Succeeded++
		case "failed":
			p.Failed++
		case "rolled_back":
			p.RolledBack++
		default:
			p.Running++
			continue
		}
		s, e1 := time.Parse(time.RFC3339, m.StartedAt)
		f, e2 := time.Parse(time.RFC3339, m.FinishedAt)
		if e1 == nil && e2 == nil && f.After(s) {
			spent += f.Sub(s)
			timed++
		}
	}
	settled := p.Succeeded + p.Failed + p.RolledBack
	if p.Total > 0 {
		p.Percent = settled * 100 / p.Total
	}
	p.State = drainState(in, p, now)
	if p.State == DrainDone && p.Total == 0 {
		p.Percent = 100
	}
	if p.State == DrainRunning && timed > 0 && p.Running > 0 {
		p.EtaSeconds = int((spent / time.Duration(timed)).Seconds()) * p.Running
	}
	return p
}

func drainState(in DrainInput, p DrainProgress, now time.Time) string {
	switch in.Status {
	case "running":
		if u, err := time.Parse(time.RFC3339, in.UpdatedAt); err == nil && in.StallAfter > 0 && now.Sub(u) > in.StallAfter {
			return DrainStalled
		}
		return DrainRunning
	case "partial":
		return DrainPartial
	case "failed":
		return DrainFailed
	case "succeeded", "done":
		return DrainDone
	}
	if p.Running > 0 {
		return DrainRunning
	}
	return DrainDone
}
