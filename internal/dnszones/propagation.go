package dnszones

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

// PropagationAnswer is one server's answer plus how it compares.
type PropagationAnswer struct {
	Answer
	Source  string `json:"source"`
	Matches *bool  `json:"matches,omitempty"`
}

// Propagation is a record's answer across resolvers and the zone's own servers.
type Propagation struct {
	Name     string              `json:"name"`
	Type     string              `json:"type"`
	Expected []string            `json:"expected,omitempty"`
	Answers  []PropagationAnswer `json:"answers"`
	Agree    bool                `json:"agree"`
}

// Answer sources.
const (
	SourceResolver      = "resolver"
	SourceAuthoritative = "authoritative"
)

// CheckPropagation asks resolvers and, when known, the zone's authoritative
// servers directly. expected, when set, is what the zone says the answer is.
func CheckPropagation(ctx context.Context, q Querier, name, typ string, resolvers, authoritative, expected []string) (Propagation, error) {
	qt, err := QType(typ)
	if err != nil {
		return Propagation{}, err
	}
	type job struct{ server, source string }
	jobs := make([]job, 0, len(resolvers)+len(authoritative))
	for _, s := range resolvers {
		jobs = append(jobs, job{withPort(s), SourceResolver})
	}
	for _, s := range authoritative {
		jobs = append(jobs, job{withPort(s), SourceAuthoritative})
	}
	out := make([]PropagationAnswer, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = PropagationAnswer{Answer: q.Query(ctx, j.server, name, qt), Source: j.source}
		}()
	}
	wg.Wait()
	p := Propagation{Name: NormalizeDomain(name), Type: strings.ToUpper(typ), Expected: sortedCopy(expected), Answers: out}
	p.Agree = annotate(p.Answers, p.Expected)
	return p, nil
}

func sortedCopy(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := slices.Clone(in)
	slices.Sort(out)
	return out
}

// annotate sets Matches on each answer and reports whether every answering
// server returned the same set.
func annotate(answers []PropagationAnswer, expected []string) bool {
	var ref []string
	seen := false
	agree := true
	for i := range answers {
		a := &answers[i]
		if a.Error != "" {
			continue
		}
		got := sortedCopy(a.Values)
		if len(expected) > 0 {
			m := slices.Equal(got, expected)
			a.Matches = &m
		}
		if !seen {
			ref, seen = got, true
			continue
		}
		if !slices.Equal(ref, got) {
			agree = false
		}
	}
	return seen && agree
}

// CommonNames are probed when importing a zone from its current servers,
// since AXFR is almost always refused.
var CommonNames = []string{
	Apex, "www", "mail", "smtp", "imap", "pop", "webmail", "ftp", "api", "app", "blog", "shop",
	"cdn", "static", "admin", "dev", "staging", "status", "docs", "m", "autodiscover", "autoconfig",
	"_dmarc", "default._domainkey", "google._domainkey", "selector1._domainkey", "selector2._domainkey",
	"k1._domainkey", "s1._domainkey", "s2._domainkey",
}

// CommonSRVNames are probed for SRV records only.
var CommonSRVNames = []string{"_sip._tls", "_sipfederationtls._tcp", "_autodiscover._tcp", "_submission._tcp", "_imaps._tcp", "_caldavs._tcp"}

var discoverTypes = []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeMX, dns.TypeTXT}

// Discover resolves common names (plus extra) against servers and returns the
// record sets it found, for an operator to review before importing.
func Discover(ctx context.Context, q Querier, zone string, extra, servers []string) []RecordSet {
	z := NormalizeDomain(zone)
	names := slices.Clone(CommonNames)
	for _, e := range extra {
		if r := RelativeName(e, z); r != "" && !slices.Contains(names, r) {
			names = append(names, r)
		}
	}
	type probe struct {
		name  string
		types []uint16
	}
	var probes []probe
	for _, n := range names {
		types := slices.Clone(discoverTypes)
		if n == Apex {
			types = append(types, dns.TypeCAA)
		} else {
			types = append([]uint16{dns.TypeCNAME}, types...)
		}
		probes = append(probes, probe{n, types})
	}
	for _, n := range CommonSRVNames {
		probes = append(probes, probe{n, []uint16{dns.TypeSRV}})
	}

	results := make([][]RecordSet, len(probes))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, p := range probes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = discoverName(ctx, q, z, p.name, p.types, servers)
		}()
	}
	wg.Wait()
	var out []RecordSet
	for _, r := range results {
		out = append(out, r...)
	}
	SortSets(out)
	return out
}

func discoverName(ctx context.Context, q Querier, zone, name string, types []uint16, servers []string) []RecordSet {
	var out []RecordSet
	for _, t := range types {
		a := firstAnswer(ctx, q, FQDN(name, zone), t, servers)
		if len(a.Values) == 0 {
			continue
		}
		out = append(out, RecordSet{Name: name, Type: dns.TypeToString[t], TTL: int(a.TTL), Values: a.Values})
		if t == dns.TypeCNAME {
			return out
		}
	}
	return out
}

func firstAnswer(ctx context.Context, q Querier, fqdn string, t uint16, servers []string) Answer {
	var last Answer
	for _, s := range servers {
		last = q.Query(ctx, withPort(s), fqdn, t)
		if last.Error == "" {
			return last
		}
	}
	return last
}
