// Package privnet models VPC-like private networks and security-group style
// allow rules between apps, and answers which app pairs can talk.
package privnet

import (
	"sort"
)

// MaxBlockedEdges caps how many blocked pairs Evaluate returns.
const MaxBlockedEdges = 200

// Edge reasons.
const (
	ViaNetwork = "network"
	ViaRule    = "rule"
	ViaNone    = ""
)

// App is the slice of an app the evaluator needs.
type App struct {
	ID        string
	Name      string
	ProjectID string
	NodeID    string
}

// Network is a private network and its member app IDs.
type Network struct {
	ID          string
	Name        string
	DefaultDeny bool
	Members     []string
}

// Rule allows two apps to reach each other across network boundaries.
type Rule struct {
	ID   string
	From string
	To   string
}

// Edge is the verdict for one unordered app pair (A sorts before B).
type Edge struct {
	A         string
	B         string
	Allowed   bool
	Via       string
	ViaName   string
	RuleID    string
	CrossNode bool
}

// Evaluate returns every allowed pair plus the blocked pairs worth showing:
// apps in the same project where at least one belongs to a private network.
func Evaluate(apps []App, nets []Network, rules []Rule) []Edge {
	member := map[string]bool{}
	shared := map[[2]string]string{}
	for _, n := range nets {
		for _, a := range n.Members {
			member[a] = true
		}
		if n.DefaultDeny {
			continue
		}
		for i, a := range n.Members {
			for _, b := range n.Members[i+1:] {
				k := pairKey(a, b)
				if _, ok := shared[k]; !ok {
					shared[k] = n.Name
				}
			}
		}
	}
	ruled := map[[2]string]string{}
	for _, r := range rules {
		ruled[pairKey(r.From, r.To)] = r.ID
	}

	byID := make(map[string]App, len(apps))
	for _, a := range apps {
		byID[a.ID] = a
	}
	ids := make([]string, 0, len(apps))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var allowed, blocked []Edge
	for i, a := range ids {
		for _, b := range ids[i+1:] {
			k := pairKey(a, b)
			e := Edge{A: a, B: b, CrossNode: byID[a].NodeID != byID[b].NodeID}
			if name, ok := shared[k]; ok {
				e.Allowed, e.Via, e.ViaName = true, ViaNetwork, name
				allowed = append(allowed, e)
				continue
			}
			if rid, ok := ruled[k]; ok {
				e.Allowed, e.Via, e.RuleID = true, ViaRule, rid
				allowed = append(allowed, e)
				continue
			}
			if byID[a].ProjectID == byID[b].ProjectID && (member[a] || member[b]) {
				blocked = append(blocked, e)
			}
		}
	}
	if len(blocked) > MaxBlockedEdges {
		blocked = blocked[:MaxBlockedEdges]
	}
	return append(allowed, blocked...)
}

func pairKey(a, b string) [2]string {
	if a > b {
		a, b = b, a
	}
	return [2]string{a, b}
}

// Link is one Docker network the enforcer keeps and the apps attached to it.
type Link struct {
	Name string
	Apps []string
}

// Links returns the Docker networks that should exist: one per private
// network (members attached unless default-deny) and one per rule.
func Links(prefix string, nets []Network, rules []Rule) []Link {
	out := make([]Link, 0, len(nets)+len(rules))
	for _, n := range nets {
		l := Link{Name: NetworkDockerName(prefix, n.ID)}
		if !n.DefaultDeny {
			l.Apps = append(l.Apps, n.Members...)
		}
		out = append(out, l)
	}
	for _, r := range rules {
		out = append(out, Link{Name: RuleDockerName(prefix, r.ID), Apps: []string{r.From, r.To}})
	}
	return out
}

// NetworkDockerName is the Docker network backing a private network.
func NetworkDockerName(prefix, id string) string { return prefix + "-pn-" + id }

// RuleDockerName is the Docker network backing an allow rule.
func RuleDockerName(prefix, id string) string { return prefix + "-pl-" + id }

// MeshHidden returns app IDs that must not be published to mesh DNS: members
// of a default-deny network with no allow rule that names them.
func MeshHidden(nets []Network, rules []Rule) map[string]bool {
	ruled := map[string]bool{}
	for _, r := range rules {
		ruled[r.From], ruled[r.To] = true, true
	}
	hidden := map[string]bool{}
	for _, n := range nets {
		if !n.DefaultDeny {
			continue
		}
		for _, a := range n.Members {
			if !ruled[a] {
				hidden[a] = true
			}
		}
	}
	return hidden
}
