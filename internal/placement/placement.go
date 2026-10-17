// Package placement is the multi-node scheduler: label selectors, a fit
// check against node CPU, memory and disk headroom, then a spread that
// prefers the most headroom. No bin-packing and no affinity rules, by design.
package placement

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ErrNoEligibleNode is returned when no candidate passes every filter.
var ErrNoEligibleNode = errors.New("placement: no eligible node")

// Candidate is one node as the scheduler sees it.
type Candidate struct {
	ID          string
	Name        string
	Labels      map[string]string
	Schedulable bool
	Online      bool
	// Load is how many apps and databases already run there.
	Load int
	// Capacity is what the node reported; Requested is what workloads
	// already placed on it asked for.
	Capacity  Capacity
	Requested Resources
}

// Selector is a set of key=value pairs a node's labels must all carry.
type Selector map[string]string

var (
	labelKeyRE   = regexp.MustCompile(`^[a-z0-9]([a-z0-9._/-]{0,61}[a-z0-9])?$`)
	labelValueRE = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]{0,61}[A-Za-z0-9])?$`)
)

// maxLabels bounds a label set so a node or selector row stays small.
const maxLabels = 32

// ValidateLabels checks keys and values are short, plain identifiers.
func ValidateLabels(labels map[string]string) error {
	if len(labels) > maxLabels {
		return fmt.Errorf("at most %d labels are allowed, got %d", maxLabels, len(labels))
	}
	for k, v := range labels {
		if !labelKeyRE.MatchString(k) {
			return fmt.Errorf("label key %q must be lowercase letters, digits, '.', '_', '-' or '/', 1 to 63 characters", k)
		}
		if !labelValueRE.MatchString(v) {
			return fmt.Errorf("label %q value %q must be letters, digits, '.', '_' or '-', 1 to 63 characters", k, v)
		}
	}
	return nil
}

// ParseLabels turns ["k=v", ...] into a validated map.
func ParseLabels(pairs []string) (map[string]string, error) {
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return nil, fmt.Errorf("label %q must be key=value", p)
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if err := ValidateLabels(out); err != nil {
		return nil, err
	}
	return out, nil
}

// Matches reports whether labels satisfy every pair in sel.
func (sel Selector) Matches(labels map[string]string) bool {
	for k, v := range sel {
		if got, ok := labels[k]; !ok || got != v {
			return false
		}
	}
	return true
}

// String renders sel as sorted "k=v,k=v".
func (sel Selector) String() string {
	parts := make([]string, 0, len(sel))
	for k, v := range sel {
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// Request is what one placement decision needs.
type Request struct {
	Selector Selector
	// Exclude is skipped outright, typically the node being drained.
	Exclude string
	// Needs are the workload's total requests; zero asks for nothing.
	Needs  Resources
	Policy Policy
	// Prefer wins when it is eligible, e.g. the node that serves ingress.
	Prefer string
}

// Rejection says why one candidate was filtered out.
type Rejection struct {
	NodeID string `json:"node_id"`
	Reason string `json:"reason"`
}

// Decision is the chosen node and the full account of how it was chosen.
type Decision struct {
	NodeID     string       `json:"node_id"`
	Why        string       `json:"why"`
	Needs      Resources    `json:"needs"`
	Policy     Policy       `json:"policy"`
	Considered []Evaluation `json:"considered"`
	Rejected   []Rejection  `json:"rejected"`
	Warnings   []string     `json:"warnings,omitempty"`
}

// CapacityBlocked reports whether any node was refused for lack of room.
func (d Decision) CapacityBlocked() bool {
	for _, e := range d.Considered {
		if e.Capacity {
			return true
		}
	}
	return false
}

// Filter returns why c may not take req, or "" when it may.
func Filter(c Candidate, req Request) string {
	switch {
	case c.ID == req.Exclude:
		return "excluded"
	case !c.Schedulable:
		return "cordoned"
	case !c.Online:
		return "offline"
	case !req.Selector.Matches(c.Labels):
		return "labels do not match " + req.Selector.String()
	}
	return ""
}

// Spread filters out nodes that cannot take req, then picks the one that
// keeps the most headroom. Nodes whose capacity is unknown rank after every
// verified node; ties fall to load, then node id, so the choice is stable.
func Spread(cands []Candidate, req Request) (Decision, error) {
	d := newDecision(cands, req)
	ranked := rank(d.Considered)
	if len(ranked) == 0 {
		return d, fmt.Errorf("%w: %s", ErrNoEligibleNode, summarize(d.Rejected))
	}
	prefer := false
	for i, e := range ranked {
		if req.Prefer != "" && e.NodeID == req.Prefer {
			ranked[0], ranked[i] = ranked[i], ranked[0]
			prefer = true
			break
		}
	}
	d.NodeID = ranked[0].NodeID
	d.Why = explainWinner(ranked, prefer)
	if ranked[0].Unverified {
		d.Warnings = append(d.Warnings, "capacity is unknown on the chosen node, so the fit could not be verified")
	}
	return d, nil
}

// Pinned records an operator's explicit choice. It is honored even when the
// node lacks room, and the shortfall is carried as a warning.
func Pinned(cands []Candidate, req Request, nodeID string) Decision {
	d := newDecision(cands, req)
	d.NodeID = nodeID
	d.Why = "pinned by the operator"
	for _, e := range d.Considered {
		if e.NodeID == nodeID && !e.Eligible {
			d.Warnings = append(d.Warnings, "the pinned node does not currently fit: "+e.Reason)
		}
	}
	return d
}

func newDecision(cands []Candidate, req Request) Decision {
	d := Decision{Needs: req.Needs, Policy: req.Policy.withDefaults(), Considered: make([]Evaluation, 0, len(cands))}
	for _, c := range cands {
		e := Evaluate(c, req)
		d.Considered = append(d.Considered, e)
		if !e.Eligible {
			d.Rejected = append(d.Rejected, Rejection{NodeID: c.ID, Reason: e.Reason})
		}
	}
	return d
}

// Check validates an explicit target against req, the manual-assignment path.
func Check(c Candidate, req Request) error {
	if reason := Filter(c, req); reason != "" {
		return fmt.Errorf("%w: node %s is %s", ErrNoEligibleNode, displayName(c), reason)
	}
	return nil
}

func displayName(c Candidate) string {
	if c.Name != "" {
		return c.Name
	}
	return c.ID
}

func summarize(rej []Rejection) string {
	if len(rej) == 0 {
		return "no nodes are registered"
	}
	parts := make([]string, 0, len(rej))
	for _, r := range rej {
		if r.Reason == "excluded" {
			continue
		}
		parts = append(parts, r.NodeID+": "+r.Reason)
	}
	if len(parts) == 0 {
		return "no other node is registered"
	}
	return strings.Join(parts, "; ")
}
