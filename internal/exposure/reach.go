package exposure

import (
	"fmt"
	"net/url"
	"sort"
)

// ReachKind names one kind of internet-reachability finding.
type ReachKind string

// Reachability finding kinds.
const (
	ReachAppDirectPort  ReachKind = "app_direct_port"
	ReachAppNoShield    ReachKind = "app_no_shield"
	ReachDatabasePublic ReachKind = "database_public"
	ReachPortExposed    ReachKind = "port_exposed"
	ReachDockerAPI      ReachKind = "docker_api"
)

// Subject types a finding or endpoint list belongs to.
const (
	SubjectApp      = "app"
	SubjectDatabase = "database"
	SubjectHost     = "host"
)

// Fix kinds: the control plane applies an auto fix, a guided fix links to the page that does it.
const (
	ReachFixAuto   = "auto"
	ReachFixGuided = "guided"
)

// ReachFix is the typed remedy for one finding. An auto fix is a call the
// dashboard makes with the operator's own session; a guided fix is a link.
type ReachFix struct {
	Kind   string            `json:"kind"`
	Method string            `json:"method,omitempty"`
	Path   string            `json:"path,omitempty"`
	Body   map[string]string `json:"body,omitempty"`
	Link   string            `json:"link,omitempty"`
	// Restarts is true when applying the fix recreates a container.
	Restarts bool `json:"restarts,omitempty"`
}

// ReachRef points at the app, database or host a finding is about.
type ReachRef struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

// ReachFinding is one thing the internet can reach that deserves a look.
type ReachFinding struct {
	ID       string            `json:"id"`
	Kind     ReachKind         `json:"kind"`
	Severity Severity          `json:"severity"`
	Subject  ReachRef          `json:"subject"`
	Node     string            `json:"node,omitempty"`
	Port     int               `json:"port,omitempty"`
	Params   map[string]string `json:"params,omitempty"`
	Fix      *ReachFix         `json:"fix,omitempty"`
}

// ReachEndpoint is one way into a subject from outside.
type ReachEndpoint struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Class Class  `json:"class,omitempty"`
}

// ReachSubject lists everything reachable for one app or database.
type ReachSubject struct {
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Endpoints []ReachEndpoint `json:"endpoints"`
	Findings  int             `json:"findings"`
}

// ReachCounts is the number of findings per severity.
type ReachCounts struct {
	High   int `json:"high"`
	Medium int `json:"medium"`
	Low    int `json:"low"`
	Info   int `json:"info"`
}

// ReachReport is the "what is reachable from the internet" answer.
type ReachReport struct {
	GeneratedAt string         `json:"generated_at"`
	Counts      ReachCounts    `json:"counts"`
	Subjects    []ReachSubject `json:"subjects"`
	Findings    []ReachFinding `json:"findings"`
	// RulesReadable is false when a node's firewall could not be read, so restricted ports may be missing.
	RulesReadable bool `json:"rules_readable"`
	// RestrictedPorts is how many published ports a firewall rule narrows.
	RestrictedPorts int `json:"restricted_ports"`
}

// ReachApp is an app's public names and the domains that have no shield.
type ReachApp struct {
	Name       string
	Domains    []string
	Unshielded []string
}

// ReachDB is a database with a published port, if any.
type ReachDB struct {
	Name       string
	PublicPort int
}

// ReachNode is one node's audit result.
type ReachNode struct {
	Name          string
	Local         bool
	RulesReadable bool
	Findings      []Finding
}

// ReachInput is everything BuildReach reads.
type ReachInput struct {
	Now       string
	Apps      []ReachApp
	Databases []ReachDB
	Nodes     []ReachNode
}

type reachKey struct{ typ, name string }

// BuildReach merges the port audit with each app's domains and shield state
// and each database's published port into one report, with a typed fix per finding.
func BuildReach(in ReachInput) ReachReport {
	rep := ReachReport{GeneratedAt: in.Now, RulesReadable: true, Subjects: []ReachSubject{}, Findings: []ReachFinding{}}
	subjects := map[reachKey]*ReachSubject{}
	subject := func(typ, name string) *ReachSubject {
		k := reachKey{typ, name}
		if s, ok := subjects[k]; ok {
			return s
		}
		s := &ReachSubject{Type: typ, Name: name, Endpoints: []ReachEndpoint{}}
		subjects[k] = s
		return s
	}
	add := func(f ReachFinding) {
		rep.Findings = append(rep.Findings, f)
		subject(f.Subject.Type, f.Subject.Name).Findings++
	}

	for _, a := range in.Apps {
		s := subject(SubjectApp, a.Name)
		for _, d := range a.Domains {
			s.Endpoints = append(s.Endpoints, ReachEndpoint{Kind: "domain", Value: d})
		}
		for _, d := range a.Unshielded {
			add(ReachFinding{
				ID: fmt.Sprintf("%s:%s:%s", ReachAppNoShield, a.Name, d), Kind: ReachAppNoShield, Severity: SeverityLow,
				Subject: ReachRef{Type: SubjectApp, Name: a.Name}, Params: map[string]string{"domain": d},
				Fix: &ReachFix{Kind: ReachFixAuto, Method: "POST", Path: "/api/v1/apps/" + url.PathEscape(a.Name) + "/shield/recommended"},
			})
		}
	}
	for _, d := range in.Databases {
		subject(SubjectDatabase, d.Name)
	}

	for _, n := range in.Nodes {
		if !n.RulesReadable {
			rep.RulesReadable = false
		}
		for _, f := range n.Findings {
			if f.Class == ClassRestricted {
				rep.RestrictedPorts++
			}
			if f.Class == ClassLoopback || f.Class == ClassPrivate {
				continue
			}
			if f.Owner.Kind == OwnerApp || f.Owner.Kind == OwnerDatabase {
				typ := SubjectApp
				if f.Owner.Kind == OwnerDatabase {
					typ = SubjectDatabase
				}
				s := subject(typ, f.Owner.Name)
				s.Endpoints = append(s.Endpoints, ReachEndpoint{Kind: "port", Value: fmt.Sprintf("%s/%d", f.Protocol, f.HostPort), Class: f.Class})
			}
			if rf, ok := reachFromPort(n, f); ok {
				add(rf)
			}
		}
	}

	for _, s := range subjects {
		rep.Subjects = append(rep.Subjects, *s)
	}
	sort.Slice(rep.Subjects, func(i, j int) bool {
		a, b := rep.Subjects[i], rep.Subjects[j]
		if a.Findings != b.Findings {
			return a.Findings > b.Findings
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.Name < b.Name
	})
	sort.SliceStable(rep.Findings, func(i, j int) bool { return sevRank(rep.Findings[i].Severity) < sevRank(rep.Findings[j].Severity) })
	for _, f := range rep.Findings {
		switch f.Severity {
		case SeverityHigh:
			rep.Counts.High++
		case SeverityMedium:
			rep.Counts.Medium++
		case SeverityLow:
			rep.Counts.Low++
		default:
			rep.Counts.Info++
		}
	}
	return rep
}

func sevRank(s Severity) int {
	switch s {
	case SeverityHigh:
		return 0
	case SeverityMedium:
		return 1
	case SeverityLow:
		return 2
	}
	return 3
}

const firewallLink = "/settings/firewall"

func reachFromPort(n ReachNode, f Finding) (ReachFinding, bool) {
	if !f.NeedsAttention() || f.Severity == SeverityInfo {
		return ReachFinding{}, false
	}
	out := ReachFinding{
		Severity: f.Severity, Node: n.Name, Port: f.HostPort,
		Params: map[string]string{"port": fmt.Sprintf("%s/%d", f.Protocol, f.HostPort), "container": f.Container},
	}
	id := func(kind ReachKind, name string) string {
		return fmt.Sprintf("%s:%s:%s:%s/%d", kind, n.Name, name, f.Protocol, f.HostPort)
	}
	switch {
	case f.ImageKind == KindDockerAPI:
		out.Kind, out.Subject = ReachDockerAPI, ReachRef{Type: SubjectHost, Name: n.Name}
		out.ID = id(out.Kind, n.Name)
		out.Fix = &ReachFix{Kind: ReachFixGuided, Link: firewallLink}
	case f.Owner.Kind == OwnerApp:
		out.Kind, out.Subject = ReachAppDirectPort, ReachRef{Type: SubjectApp, Name: f.Owner.Name}
		out.ID = id(out.Kind, f.Owner.Name)
		if n.Local {
			out.Fix = &ReachFix{Kind: ReachFixAuto, Method: "PUT", Path: "/api/v1/apps/" + url.PathEscape(f.Owner.Name) + "/exposure",
				Body: map[string]string{"mode": "proxied"}, Restarts: true}
		} else {
			out.Fix = &ReachFix{Kind: ReachFixGuided, Link: "/apps/" + url.PathEscape(f.Owner.Name)}
		}
	case f.Owner.Kind == OwnerDatabase:
		out.Kind, out.Subject = ReachDatabasePublic, ReachRef{Type: SubjectDatabase, Name: f.Owner.Name}
		out.ID = id(out.Kind, f.Owner.Name)
		out.Fix = &ReachFix{Kind: ReachFixGuided, Link: "/databases/" + url.PathEscape(f.Owner.Name)}
	default:
		out.Kind, out.Subject = ReachPortExposed, ReachRef{Type: SubjectHost, Name: n.Name}
		out.ID = id(out.Kind, f.Container)
		out.Fix = &ReachFix{Kind: ReachFixGuided, Link: firewallLink}
	}
	return out, true
}
