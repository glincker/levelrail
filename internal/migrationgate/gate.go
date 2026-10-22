// Package migrationgate decides whether the old instance is safe to delete.
// It is pure: callers pass in what the migration job recorded and what the
// live probes saw, and a gate only passes on evidence, never on absence of failure.
package migrationgate

import (
	"fmt"
	"sort"
	"strings"
)

// Status is the outcome of one gate.
type Status string

// Gate outcomes.
const (
	Pass Status = "pass"
	Fail Status = "fail"
	// Pending means the evidence has not been gathered yet, so the gate cannot pass.
	Pending Status = "pending"
	// Manual is a step only a person can confirm.
	Manual Status = "manual"
)

// Gate IDs, in the order they are evaluated. The last three double as the
// acknowledgment IDs a caller passes in Input.Acknowledged.
const (
	GateStaged    = "staged"
	GateLeftovers = "leftovers"
	GateVolumes   = "volumes"
	GateDatabases = "databases"
	GateRouted    = "routed"
	GateDNS       = "dns"
	GateResponds  = "responds"
	GateBackup    = "source-backup"
	GateTraffic   = "traffic-cycle"
)

// Item uses the import state strings of the job: verified and routed count as proven here.
type Item struct {
	Name    string
	State   string
	Domains []string
	Volumes []Volume
	// Leftover is true for an unsupported resource the importer cannot move.
	Leftover bool
}

// Volume is one persistent mount and whether its copy completed.
type Volume struct {
	Name   string
	Copied bool
}

// Database is one source database the selected apps use and the state of its data copy.
type Database struct {
	Name string
	// Status is the data copy status: verified, copying, failed, pending, or empty when never started.
	Status     string
	Mismatched int
}

// Probe is one live result for a domain.
type Probe struct {
	Domain   string
	DNS      bool
	Responds bool
}

// Input is everything the gates look at. Nil Probes means no probe ran yet.
type Input struct {
	Items        []Item
	Databases    []Database
	Probes       []Probe
	Acknowledged []string
}

// Gate is one line of the checklist.
type Gate struct {
	ID     string   `json:"id"`
	Status Status   `json:"status"`
	Detail string   `json:"detail"`
	Open   []string `json:"open,omitempty"`
	Fix    string   `json:"fix,omitempty"`
}

// Report is the checklist. Safe is true only when every gate passed.
type Report struct {
	Safe     bool           `json:"safe_to_delete"`
	Gates    []Gate         `json:"gates"`
	Counts   map[Status]int `json:"counts"`
	Fallback []string       `json:"fallback"`
}

// Evaluate runs every gate.
func Evaluate(in Input) Report {
	ack := map[string]bool{}
	for _, a := range in.Acknowledged {
		ack[a] = true
	}
	gates := []Gate{staged(in.Items), leftovers(in.Items, ack), volumes(in.Items), databases(in.Databases), routed(in.Items)}
	gates = append(gates, probeGates(in)...)
	gates = append(gates,
		manual(GateBackup, ack,
			"Keep a backup of the old instance's own configuration and database until the last step.",
			"Copy the source environment file and dump its own database, then confirm."),
		manual(GateTraffic, ack,
			"Run a full traffic cycle here, with scheduled jobs and backups executed at least once.",
			"Wait out one full cycle of normal traffic and your scheduled work, then confirm."))
	rep := Report{Safe: true, Gates: gates, Counts: map[Status]int{}, Fallback: Fallback()}
	for _, g := range gates {
		rep.Counts[g.Status]++
		if g.Status != Pass {
			rep.Safe = false
		}
	}
	return rep
}

func staged(items []Item) Gate {
	g := Gate{ID: GateStaged}
	var open []string
	n := 0
	for _, it := range items {
		if it.Leftover {
			continue
		}
		n++
		if it.State != "verified" && it.State != "routed" {
			open = append(open, fmt.Sprintf("%s (%s)", it.Name, it.State))
		}
	}
	switch {
	case n == 0:
		g.Status, g.Detail = Fail, "no app was imported"
		g.Fix = "select and stage the apps you want to move"
	case len(open) > 0:
		g.Status, g.Detail, g.Open = Fail, fmt.Sprintf("%d of %d app(s) are not built and healthy here", len(open), n), open
		g.Fix = "stage and verify the listed apps, or retry the ones that failed"
	default:
		g.Status, g.Detail = Pass, fmt.Sprintf("%d app(s) built and healthy here", n)
	}
	return g
}

func leftovers(items []Item, ack map[string]bool) Gate {
	g := Gate{ID: GateLeftovers}
	var open []string
	for _, it := range items {
		if it.Leftover {
			open = append(open, it.Name)
		}
	}
	sort.Strings(open)
	switch {
	case len(open) == 0:
		g.Status, g.Detail = Pass, "nothing was left behind"
	case ack[GateLeftovers]:
		g.Status, g.Detail, g.Open = Pass, fmt.Sprintf("%d resource(s) were moved by hand, as confirmed", len(open)), open
	default:
		g.Status, g.Detail, g.Open = Manual, fmt.Sprintf("%d resource(s) cannot be imported and need you", len(open)), open
		g.Fix = "move each by hand or accept losing it, then confirm"
	}
	return g
}

func volumes(items []Item) Gate {
	g := Gate{ID: GateVolumes}
	var open []string
	total := 0
	for _, it := range items {
		if it.Leftover {
			continue
		}
		for _, v := range it.Volumes {
			total++
			if !v.Copied {
				open = append(open, it.Name+": "+v.Name)
			}
		}
	}
	switch {
	case total == 0:
		g.Status, g.Detail = Pass, "no volume to copy"
	case len(open) > 0:
		g.Status, g.Detail, g.Open = Fail, fmt.Sprintf("%d of %d volume(s) are not copied", len(open), total), open
		g.Fix = "copy the volumes again right before the switch"
	default:
		g.Status, g.Detail = Pass, fmt.Sprintf("%d volume(s) copied", total)
	}
	return g
}

func databases(dbs []Database) Gate {
	g := Gate{ID: GateDatabases}
	var open []string
	for _, d := range dbs {
		switch {
		case d.Status != "verified":
			s := d.Status
			if s == "" {
				s = "not copied"
			}
			open = append(open, fmt.Sprintf("%s (%s)", d.Name, s))
		case d.Mismatched > 0:
			open = append(open, fmt.Sprintf("%s (%d table(s) differ)", d.Name, d.Mismatched))
		}
	}
	switch {
	case len(dbs) == 0:
		g.Status, g.Detail = Pass, "no database to copy"
	case len(open) > 0:
		g.Status, g.Detail, g.Open = Fail, fmt.Sprintf("%d of %d database copy(ies) are not verified", len(open), len(dbs)), open
		g.Fix = "run the data copy again and compare row counts until every table matches"
	default:
		g.Status, g.Detail = Pass, fmt.Sprintf("%d database copy(ies) verified, row counts match", len(dbs))
	}
	return g
}

func routed(items []Item) Gate {
	g := Gate{ID: GateRouted}
	var open []string
	total := 0
	for _, it := range items {
		if it.Leftover || len(it.Domains) == 0 {
			continue
		}
		total++
		if it.State != "routed" {
			open = append(open, it.Name)
		}
	}
	switch {
	case total == 0:
		g.Status, g.Detail = Pass, "no imported app has a domain"
	case len(open) > 0:
		g.Status, g.Detail, g.Open = Fail, fmt.Sprintf("%d app(s) do not serve their domains here yet", len(open)), open
		g.Fix = "enable routing for the listed apps"
	default:
		g.Status, g.Detail = Pass, fmt.Sprintf("%d app(s) serve their domains here", total)
	}
	return g
}

func probeGates(in Input) []Gate {
	var domains []string
	for _, it := range in.Items {
		if !it.Leftover {
			domains = append(domains, it.Domains...)
		}
	}
	dns, resp := Gate{ID: GateDNS}, Gate{ID: GateResponds}
	if len(domains) == 0 {
		dns.Status, dns.Detail = Pass, "no domain to switch"
		resp.Status, resp.Detail = Pass, "no domain to check"
		return []Gate{dns, resp}
	}
	byDomain := map[string]Probe{}
	for _, p := range in.Probes {
		byDomain[strings.ToLower(p.Domain)] = p
	}
	var dnsOpen, respOpen []string
	for _, d := range domains {
		p, ok := byDomain[strings.ToLower(d)]
		if !ok || !p.DNS {
			dnsOpen = append(dnsOpen, d)
		}
		if !ok || !p.Responds {
			respOpen = append(respOpen, d)
		}
	}
	fill := func(g *Gate, open []string, what, fix string) {
		switch {
		case len(open) == 0:
			g.Status, g.Detail = Pass, fmt.Sprintf("all %d domain(s) %s", len(domains), what)
		case len(in.Probes) == 0:
			g.Status, g.Detail, g.Open, g.Fix = Pending, "the live check has not run yet", open, "run the check now"
		default:
			g.Status, g.Detail, g.Open, g.Fix = Fail, fmt.Sprintf("%d of %d domain(s) are not %s", len(open), len(domains), what), open, fix
		}
	}
	fill(&dns, dnsOpen, "pointing at this server", "change the DNS record, or wait out its TTL")
	fill(&resp, respOpen, "answering from this server", "check the app's logs and the certificate")
	return []Gate{dns, resp}
}

func manual(id string, ack map[string]bool, detail, fix string) Gate {
	if ack[id] {
		return Gate{ID: id, Status: Pass, Detail: detail + " (confirmed)"}
	}
	return Gate{ID: id, Status: Manual, Detail: detail, Fix: fix}
}

// Fallback is the rollback guidance shown next to the checklist.
func Fallback() []string {
	return []string{
		"Keep the old instance running and untouched until every gate here is green.",
		"Before the first write lands here, a rollback is a DNS change: point each record at the old server again.",
		"After a database takes writes here, copy the data back to the old database before you roll back, or those writes are lost.",
		"Disable an app's scheduled jobs here when you roll it back, so a job never runs on both servers.",
		"Stop the old instance and keep it stopped for a few days before you delete it. Keep its backups.",
	}
}
