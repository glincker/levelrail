package dnszones

import (
	"context"
	"slices"
	"sync"

	"github.com/miekg/dns"
)

// Delegation states for a zone.
const (
	DelegationNone      = "not_delegated"
	DelegationPartial   = "partially_delegated"
	DelegationDelegated = "delegated"
	DelegationElsewhere = "delegated_elsewhere"
)

// Per resolver verdicts.
const (
	resolverMatch     = "match"
	resolverPartial   = "partial"
	resolverElsewhere = "elsewhere"
	resolverNone      = "none"
)

// ResolverDelegation is what one resolver says the domain's name servers are.
type ResolverDelegation struct {
	Server      string   `json:"server"`
	NameServers []string `json:"name_servers,omitempty"`
	Verdict     string   `json:"verdict"`
	Error       string   `json:"error,omitempty"`
}

// Delegation compares a zone's assigned name servers with what resolvers see.
type Delegation struct {
	Domain    string               `json:"domain"`
	State     string               `json:"state"`
	Expected  []string             `json:"expected"`
	Resolvers []ResolverDelegation `json:"resolvers"`
	Elsewhere []string             `json:"elsewhere,omitempty"`
}

// DefaultCheckServers are asked for delegation and propagation checks.
var DefaultCheckServers = []string{ServerSystem, "1.1.1.1:53", "8.8.8.8:53"}

// CheckDelegation asks each server for the domain's NS set, in parallel.
func CheckDelegation(ctx context.Context, q Querier, domain string, expected, servers []string) Delegation {
	answers := make([]Answer, len(servers))
	var wg sync.WaitGroup
	for i, s := range servers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			answers[i] = q.Query(ctx, withPort(s), domain, dns.TypeNS)
		}()
	}
	wg.Wait()
	d := ComputeDelegation(expected, answers)
	d.Domain = NormalizeDomain(domain)
	return d
}

// ComputeDelegation derives the overall state from per resolver answers.
// A resolver whose NS set is a subset of the assigned one counts as a match:
// a registrar given two of Route53's four servers still delegates correctly.
func ComputeDelegation(expected []string, answers []Answer) Delegation {
	exp := normalizeAll(expected)
	d := Delegation{Expected: exp}
	counts := map[string]int{}
	for _, a := range answers {
		got := normalizeAll(a.Values)
		rd := ResolverDelegation{Server: a.Server, NameServers: got, Error: a.Error}
		in, out := 0, 0
		for _, ns := range got {
			if slices.Contains(exp, ns) {
				in++
			} else {
				out++
				if !slices.Contains(d.Elsewhere, ns) {
					d.Elsewhere = append(d.Elsewhere, ns)
				}
			}
		}
		switch {
		case len(got) == 0:
			rd.Verdict = resolverNone
		case out == 0:
			rd.Verdict = resolverMatch
		case in == 0:
			rd.Verdict = resolverElsewhere
		default:
			rd.Verdict = resolverPartial
		}
		counts[rd.Verdict]++
		d.Resolvers = append(d.Resolvers, rd)
	}
	total := len(answers)
	switch {
	case total > 0 && counts[resolverMatch] == total:
		d.State = DelegationDelegated
	case counts[resolverMatch]+counts[resolverPartial] > 0:
		d.State = DelegationPartial
	case counts[resolverElsewhere] > 0:
		d.State = DelegationElsewhere
	default:
		d.State = DelegationNone
	}
	slices.Sort(d.Elsewhere)
	return d
}

func normalizeAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if n := NormalizeDomain(s); n != "" && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}
