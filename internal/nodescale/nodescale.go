// Package nodescale finds cloud nodes that sit idle and could be scaled in.
// This release is dry run only: it reports would_destroy events and never
// destroys anything.
package nodescale

import (
	"context"
	"sort"
	"sync"
	"time"
)

const (
	// EventWouldDestroy is the only event kind; nothing is ever destroyed here.
	EventWouldDestroy = "would_destroy"
	// LabelAutoscale opts a node in when set to LabelAutoscaleOn.
	LabelAutoscale = "autoscale"
	// LabelAutoscaleOn is the opt-in value.
	LabelAutoscaleOn = "on"
	// DefaultPool names the pool of a node that matches no pool selector.
	DefaultPool = "default"
)

// Node is what the evaluator needs to know about one node.
type Node struct {
	ID   string
	Name string
	// Pools are the pools whose selector matches the node; empty means DefaultPool.
	Pools  []string
	Labels map[string]string
	// Provisioned is true when Levelrail created the node through cloud provisioning.
	Provisioned  bool
	ControlPlane bool
	// Workloads counts apps and databases placed on the node.
	Workloads int
	// HoldsVolumes is true when the node may hold the only copy of a volume,
	// for example while a volume move to or from it is running.
	HoldsVolumes bool
	// MonthlyPrice is zero when unknown.
	MonthlyPrice float64
}

// Source lists the nodes to evaluate.
type Source interface {
	Nodes(ctx context.Context) ([]Node, error)
}

// Settings are the runtime tunables, read on every pass.
type Settings interface {
	Enabled(ctx context.Context) bool
	IdleMinutes(ctx context.Context) int
}

// Event is one would_destroy finding.
type Event struct {
	Kind         string    `json:"kind"`
	NodeID       string    `json:"node_id"`
	NodeName     string    `json:"node_name"`
	Pool         string    `json:"pool"`
	IdleMinutes  int       `json:"idle_minutes"`
	MonthlyPrice *float64  `json:"monthly_price,omitempty"`
	At           time.Time `json:"at"`
}

// Candidate is a node a real scale-in would remove.
type Candidate struct {
	NodeID       string   `json:"node_id"`
	NodeName     string   `json:"node_name"`
	Pool         string   `json:"pool"`
	IdleMinutes  int      `json:"idle_minutes"`
	MonthlyPrice *float64 `json:"monthly_price,omitempty"`
}

// Preview is the read-only answer for the API and CLI.
type Preview struct {
	Enabled           bool        `json:"enabled"`
	DryRunOnly        bool        `json:"dry_run_only"`
	IdleMinutes       int         `json:"idle_minutes"`
	Candidates        []Candidate `json:"candidates"`
	MonthlySaving     float64     `json:"monthly_saving"`
	UnpricedCandidate int         `json:"unpriced_candidates"`
}

// Worker evaluates nodes and emits events. It holds idle timers in memory, so a
// restart only delays a finding, never invents one.
type Worker struct {
	src      Source
	settings Settings
	emit     func(Event)
	interval time.Duration

	mu        sync.Mutex
	idleSince map[string]time.Time
	emitted   map[string]bool
}

// New builds a worker. emit may be nil. interval is the evaluation period.
func New(src Source, settings Settings, interval time.Duration, emit func(Event)) *Worker {
	return &Worker{src: src, settings: settings, emit: emit, interval: interval,
		idleSince: map[string]time.Time{}, emitted: map[string]bool{}}
}

// Run evaluates until ctx ends.
func (w *Worker) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			w.Tick(ctx, now)
		}
	}
}

// Tick runs one pass and returns the events it emitted. With scale_in off it does nothing.
func (w *Worker) Tick(ctx context.Context, now time.Time) []Event {
	if !w.settings.Enabled(ctx) {
		return nil
	}
	cands, err := w.evaluate(ctx, now)
	if err != nil {
		return nil
	}
	var out []Event
	w.mu.Lock()
	seen := map[string]bool{}
	for _, c := range cands {
		seen[c.NodeID] = true
		if w.emitted[c.NodeID] {
			continue
		}
		w.emitted[c.NodeID] = true
		out = append(out, Event{Kind: EventWouldDestroy, NodeID: c.NodeID, NodeName: c.NodeName, Pool: c.Pool,
			IdleMinutes: c.IdleMinutes, MonthlyPrice: c.MonthlyPrice, At: now})
	}
	for id := range w.emitted {
		if !seen[id] {
			delete(w.emitted, id)
		}
	}
	w.mu.Unlock()
	if w.emit != nil {
		for _, e := range out {
			w.emit(e)
		}
	}
	return out
}

// Preview reports the current candidates without emitting anything.
func (w *Worker) Preview(ctx context.Context, now time.Time) (Preview, error) {
	p := Preview{Enabled: w.settings.Enabled(ctx), DryRunOnly: true, IdleMinutes: w.settings.IdleMinutes(ctx), Candidates: []Candidate{}}
	cands, err := w.evaluate(ctx, now)
	if err != nil {
		return p, err
	}
	for _, c := range cands {
		p.Candidates = append(p.Candidates, c)
		if c.MonthlyPrice != nil {
			p.MonthlySaving += *c.MonthlyPrice
		} else {
			p.UnpricedCandidate++
		}
	}
	return p, nil
}

func poolsOf(n Node) []string {
	if len(n.Pools) == 0 {
		return []string{DefaultPool}
	}
	return n.Pools
}

func (w *Worker) evaluate(ctx context.Context, now time.Time) ([]Candidate, error) {
	nodes, err := w.src.Nodes(ctx)
	if err != nil {
		return nil, err
	}
	idleMin := w.settings.IdleMinutes(ctx)
	members := map[string]int{}
	for _, n := range nodes {
		for _, p := range poolsOf(n) {
			members[p]++
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	live := map[string]bool{}
	var out []Candidate
	for _, n := range nodes {
		live[n.ID] = true
		if n.Workloads > 0 {
			delete(w.idleSince, n.ID)
			continue
		}
		since, ok := w.idleSince[n.ID]
		if !ok {
			since = now
			w.idleSince[n.ID] = since
		}
		if !n.Provisioned || n.ControlPlane || n.HoldsVolumes || n.Labels[LabelAutoscale] != LabelAutoscaleOn {
			continue
		}
		lastInPool := false
		for _, p := range poolsOf(n) {
			if members[p] <= 1 {
				lastInPool = true
			}
		}
		idle := int(now.Sub(since) / time.Minute)
		if lastInPool || idle < idleMin {
			continue
		}
		c := Candidate{NodeID: n.ID, NodeName: n.Name, Pool: poolsOf(n)[0], IdleMinutes: idle}
		if n.MonthlyPrice > 0 {
			price := n.MonthlyPrice
			c.MonthlyPrice = &price
		}
		out = append(out, c)
	}
	for id := range w.idleSince {
		if !live[id] {
			delete(w.idleSince, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out, nil
}
