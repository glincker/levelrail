package placement

import (
	"fmt"
	"strings"
)

// MatchOp is how a LabelTerm compares a label to its values.
type MatchOp string

// Label term operators.
const (
	OpIn           MatchOp = "In"
	OpNotIn        MatchOp = "NotIn"
	OpExists       MatchOp = "Exists"
	OpDoesNotExist MatchOp = "DoesNotExist"
)

// TopologyNode scopes a workload term to a single node; any other topology is
// the name of a node label such as "zone".
const TopologyNode = "node"

// LabelTerm tests one label key.
type LabelTerm struct {
	Key    string   `json:"key" yaml:"key"`
	Op     MatchOp  `json:"op" yaml:"op"`
	Values []string `json:"values,omitempty" yaml:"values,omitempty"`
}

// Matches reports whether labels satisfy the term.
func (t LabelTerm) Matches(labels map[string]string) bool {
	got, ok := labels[t.Key]
	switch t.Op {
	case OpExists:
		return ok
	case OpDoesNotExist:
		return !ok
	case OpIn:
		return ok && contains(t.Values, got)
	case OpNotIn:
		return !ok || !contains(t.Values, got)
	}
	return false
}

func contains(vals []string, v string) bool {
	for _, x := range vals {
		if x == v {
			return true
		}
	}
	return false
}

func (t LabelTerm) String() string {
	switch t.Op {
	case OpExists:
		return t.Key
	case OpDoesNotExist:
		return "!" + t.Key
	}
	return t.Key + " " + strings.ToLower(string(t.Op)) + " (" + strings.Join(t.Values, ",") + ")"
}

// WeightedNodeTerm is a node label preference; the highest total weight wins.
type WeightedNodeTerm struct {
	Weight int       `json:"weight" yaml:"weight"`
	Term   LabelTerm `json:"term" yaml:"term"`
}

// WorkloadTerm selects other workloads by label and applies within a topology.
// Weight 0 makes the term required; 1 to 100 makes it a preference.
type WorkloadTerm struct {
	Match    []LabelTerm `json:"match" yaml:"match"`
	Topology string      `json:"topology,omitempty" yaml:"topology,omitempty"`
	Weight   int         `json:"weight,omitempty" yaml:"weight,omitempty"`
}

// Affinity is the optional rule set of one workload. Together attracts it to
// nodes (or zones) running matching workloads, Apart repels it.
type Affinity struct {
	NodeRequired  []LabelTerm        `json:"node_required,omitempty" yaml:"node_required,omitempty"`
	NodePreferred []WeightedNodeTerm `json:"node_preferred,omitempty" yaml:"node_preferred,omitempty"`
	Together      []WorkloadTerm     `json:"together,omitempty" yaml:"together,omitempty"`
	Apart         []WorkloadTerm     `json:"apart,omitempty" yaml:"apart,omitempty"`
}

// Empty reports whether the rule set asks for nothing.
func (a Affinity) Empty() bool {
	return len(a.NodeRequired) == 0 && len(a.NodePreferred) == 0 && len(a.Together) == 0 && len(a.Apart) == 0
}

// affinityState holds, per workload term, the topology domains that already
// run a matching workload. It needs every candidate, so Spread builds it.
type affinityState struct {
	together []map[string]bool
	apart    []map[string]bool
}

func topologyValue(c Candidate, topology string) (string, bool) {
	if topology == "" || topology == TopologyNode {
		return c.ID, true
	}
	v, ok := c.Labels[topology]
	return v, ok && v != ""
}

func matchesAll(terms []LabelTerm, labels map[string]string) bool {
	for _, t := range terms {
		if !t.Matches(labels) {
			return false
		}
	}
	return true
}

func domainsFor(cands []Candidate, terms []WorkloadTerm) []map[string]bool {
	out := make([]map[string]bool, len(terms))
	for i, term := range terms {
		set := map[string]bool{}
		for _, c := range cands {
			dom, ok := topologyValue(c, term.Topology)
			if !ok {
				continue
			}
			for _, w := range c.Workloads {
				if matchesAll(term.Match, w) {
					set[dom] = true
					break
				}
			}
		}
		out[i] = set
	}
	return out
}

func buildAffinityState(cands []Candidate, a Affinity) *affinityState {
	if len(a.Together) == 0 && len(a.Apart) == 0 {
		return nil
	}
	return &affinityState{together: domainsFor(cands, a.Together), apart: domainsFor(cands, a.Apart)}
}

func describeTerm(t WorkloadTerm) string {
	parts := make([]string, len(t.Match))
	for i, m := range t.Match {
		parts[i] = m.String()
	}
	topo := t.Topology
	if topo == "" {
		topo = TopologyNode
	}
	return strings.Join(parts, " and ") + " per " + topo
}

// checkAffinity returns why c breaks a required rule ("" when none) and the
// summed weight of the preferences c satisfies.
func checkAffinity(c Candidate, a Affinity, st *affinityState) (string, int) {
	score := 0
	for _, p := range a.NodePreferred {
		if p.Term.Matches(c.Labels) {
			score += p.Weight
		}
	}
	if st == nil {
		return "", score
	}
	for i, t := range a.Together {
		dom, ok := topologyValue(c, t.Topology)
		in := ok && st.together[i][dom]
		switch {
		case t.Weight == 0 && !in:
			return "affinity not met: no workload matching " + describeTerm(t) + " runs here", 0
		case t.Weight > 0 && in:
			score += t.Weight
		}
	}
	for i, t := range a.Apart {
		dom, ok := topologyValue(c, t.Topology)
		in := ok && st.apart[i][dom]
		switch {
		case t.Weight == 0 && in:
			return "anti-affinity: a workload matching " + describeTerm(t) + " already runs here", 0
		case t.Weight > 0 && !in:
			score += t.Weight
		}
	}
	return "", score
}

// ValidateAffinity checks operators, keys, weights and the number of terms.
// maxTerms comes from settings so the bound is not hard coded.
func ValidateAffinity(a Affinity, maxTerms int) error {
	total := len(a.NodeRequired) + len(a.NodePreferred) + len(a.Together) + len(a.Apart)
	if maxTerms > 0 && total > maxTerms {
		return fmt.Errorf("at most %d affinity terms are allowed, got %d", maxTerms, total)
	}
	check := func(t LabelTerm) error {
		if !labelKeyRE.MatchString(t.Key) {
			return fmt.Errorf("affinity key %q must be lowercase letters, digits, '.', '_', '-' or '/', 1 to 63 characters", t.Key)
		}
		switch t.Op {
		case OpIn, OpNotIn:
			if len(t.Values) == 0 {
				return fmt.Errorf("affinity term %q needs at least one value", t.Key)
			}
			for _, v := range t.Values {
				if !labelValueRE.MatchString(v) {
					return fmt.Errorf("affinity term %q value %q must be letters, digits, '.', '_' or '-', 1 to 63 characters", t.Key, v)
				}
			}
		case OpExists, OpDoesNotExist:
			if len(t.Values) > 0 {
				return fmt.Errorf("affinity term %q takes no values with %s", t.Key, t.Op)
			}
		default:
			return fmt.Errorf("affinity operator %q must be In, NotIn, Exists or DoesNotExist", t.Op)
		}
		return nil
	}
	for _, t := range a.NodeRequired {
		if err := check(t); err != nil {
			return err
		}
	}
	for _, p := range a.NodePreferred {
		if p.Weight < 1 || p.Weight > 100 {
			return fmt.Errorf("preferred node term %q needs a weight of 1 to 100, got %d", p.Term.Key, p.Weight)
		}
		if err := check(p.Term); err != nil {
			return err
		}
	}
	for _, group := range [][]WorkloadTerm{a.Together, a.Apart} {
		for _, t := range group {
			if len(t.Match) == 0 {
				return fmt.Errorf("a workload affinity term needs at least one match")
			}
			if t.Weight < 0 || t.Weight > 100 {
				return fmt.Errorf("workload term weight must be 0 (required) or 1 to 100, got %d", t.Weight)
			}
			if t.Topology != "" && t.Topology != TopologyNode && !labelKeyRE.MatchString(t.Topology) {
				return fmt.Errorf("topology %q must be %q or a node label key", t.Topology, TopologyNode)
			}
			for _, m := range t.Match {
				if err := check(m); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
