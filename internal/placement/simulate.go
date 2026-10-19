package placement

import (
	"errors"
	"sort"
	"strconv"
)

// Placed is one simulated workload and where the scheduler would put it.
type Placed struct {
	Name     string `json:"name"`
	From     string `json:"from_node_id,omitempty"`
	NodeID   string `json:"node_id,omitempty"`
	NodeName string `json:"node_name,omitempty"`
	Zone     string `json:"zone,omitempty"`
	Why      string `json:"why,omitempty"`
	// Unplaced carries the refusal when no node could take the workload.
	Unplaced string `json:"unplaced,omitempty"`
}

// Simulation is a dry run: nothing is written, the candidates are copied.
type Simulation struct {
	Placed   []Placed      `json:"placed"`
	Unplaced int           `json:"unplaced"`
	Zones    []ZoneSummary `json:"zones"`
}

// Workload is something already running that a failure would displace.
type Workload struct {
	Name   string
	NodeID string
	Needs  Resources
	// Selector is the label constraint the workload was placed under.
	Selector Selector
}

// Simulate places count identical workloads one after another, each seeing the
// load the previous ones added, exactly as real creations would.
func Simulate(cands []Candidate, req Request, count int) Simulation {
	work := cloneCandidates(cands)
	out := Simulation{Placed: make([]Placed, 0, max(count, 0))}
	for i := 1; i <= count; i++ {
		p := Placed{Name: "app-" + strconv.Itoa(i)}
		d, err := Spread(work, req)
		if err != nil {
			p.Unplaced = refusal(d, err)
			out.Unplaced++
		} else {
			assign(work, &p, d, req.Needs)
		}
		out.Placed = append(out.Placed, p)
	}
	out.Zones = Zones(work)
	return out
}

// FailNodes simulates losing the nodes for which dead returns true: every
// workload on them is rescheduled onto survivors in a stable order (largest
// memory first, then name) so the same fleet always gives the same answer.
func FailNodes(cands []Candidate, workloads []Workload, dead func(Candidate) bool) Simulation {
	work := cloneCandidates(cands)
	lost := map[string]bool{}
	for i := range work {
		if dead(work[i]) {
			lost[work[i].ID] = true
			work[i].Online = false
			work[i].Load, work[i].Requested = 0, Resources{}
		}
	}
	var moving []Workload
	for _, w := range workloads {
		if lost[w.NodeID] {
			moving = append(moving, w)
		}
	}
	sort.SliceStable(moving, func(i, j int) bool {
		a, b := moving[i], moving[j]
		if a.Needs.MemoryBytes != b.Needs.MemoryBytes {
			return a.Needs.MemoryBytes > b.Needs.MemoryBytes
		}
		return a.Name < b.Name
	})
	out := Simulation{Placed: make([]Placed, 0, len(moving))}
	for _, w := range moving {
		p := Placed{Name: w.Name, From: w.NodeID}
		d, err := Spread(work, Request{Selector: w.Selector, Needs: w.Needs, Exclude: w.NodeID})
		if err != nil {
			p.Unplaced = refusal(d, err)
			out.Unplaced++
		} else {
			assign(work, &p, d, w.Needs)
		}
		out.Placed = append(out.Placed, p)
	}
	out.Zones = Zones(work)
	return out
}

func assign(work []Candidate, p *Placed, d Decision, needs Resources) {
	p.NodeID, p.Why = d.NodeID, d.Why
	for i := range work {
		if work[i].ID == d.NodeID {
			work[i].Load++
			work[i].Requested = work[i].Requested.Add(needs)
			p.NodeName, p.Zone = work[i].Name, ZoneOf(work[i].Labels)
		}
	}
}

func refusal(d Decision, err error) string {
	if errors.Is(err, ErrNoEligibleNode) {
		return err.Error()
	}
	return d.Why
}

func cloneCandidates(in []Candidate) []Candidate {
	out := make([]Candidate, len(in))
	copy(out, in)
	return out
}
