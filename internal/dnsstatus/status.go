// Package dnsstatus reports, for each DNS record a feature needs, whether it is
// live, still waiting, different or blocked, and what the zone already holds.
package dnsstatus

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"
)

// Record statuses.
const (
	Live      = "live"
	Waiting   = "waiting"
	Different = "different"
	Conflict  = "conflict"
	Unknown   = "unknown"
)

// Where the found records came from.
const (
	SourceProvider  = "provider"
	SourcePublicDNS = "public_dns"
)

// Want is one record a feature needs. Name is absolute without a trailing dot;
// MX and SRV carry the priority separately from Value.
type Want struct {
	ID       string
	Type     string
	Name     string
	Value    string
	TTL      int
	Priority *int
	// Reach adds names that must stay unproxied for this record to work (mta-sts).
	Reach []string
}

// Found is one record that already exists at the name.
type Found struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Value   string `json:"value"`
	TTL     int    `json:"ttl,omitempty"`
	Proxied *bool  `json:"proxied,omitempty"`
}

// Record is the live status of one wanted record.
type Record struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"`
	Name      string  `json:"name"`
	Value     string  `json:"value"`
	TTL       int     `json:"ttl"`
	Priority  *int    `json:"priority,omitempty"`
	Status    string  `json:"status"`
	Found     []Found `json:"found"`
	Reason    string  `json:"reason,omitempty"`
	CheckedAt string  `json:"checked_at"`
}

// Counts tallies records by status.
type Counts struct {
	Live      int `json:"live"`
	Waiting   int `json:"waiting"`
	Different int `json:"different"`
	Conflict  int `json:"conflict"`
	Unknown   int `json:"unknown"`
	Total     int `json:"total"`
}

// Status is the per-record report plus its counts and source.
type Status struct {
	ProviderSource string   `json:"provider_source"`
	CheckedAt      string   `json:"checked_at"`
	Counts         Counts   `json:"counts"`
	Records        []Record `json:"records"`
}

func (c *Counts) add(status string) {
	c.Total++
	switch status {
	case Live:
		c.Live++
	case Waiting:
		c.Waiting++
	case Different:
		c.Different++
	case Conflict:
		c.Conflict++
	default:
		c.Unknown++
	}
}

func newStatus(source string, recs []Record, now time.Time) Status {
	s := Status{ProviderSource: source, CheckedAt: now.UTC().Format(time.RFC3339), Records: recs}
	if s.Records == nil {
		s.Records = []Record{}
	}
	for i := range s.Records {
		s.Records[i].CheckedAt = s.CheckedAt
		s.Counts.add(s.Records[i].Status)
	}
	return s
}

// JoinTXT rejoins a TXT value written as quoted strings (DKIM and SPF are often
// split into 255 byte chunks) into the single string receivers see.
func JoinTXT(v string) string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, `"`) {
		return v
	}
	var b strings.Builder
	in, esc := false, false
	for _, r := range v {
		switch {
		case esc:
			b.WriteRune(r)
			esc = false
		case in && r == '\\':
			esc = true
		case r == '"':
			in = !in
		case in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func host(v string) string { return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(v)), ".") }

// Normalize is the comparable form of a record value: case, trailing dots,
// quotes, TXT chunks and spacing do not make two values differ.
func Normalize(typ, v string) string {
	switch typ {
	case "TXT":
		return strings.ToLower(strings.Join(strings.Fields(JoinTXT(v)), " "))
	case "A", "AAAA":
		if a, err := netip.ParseAddr(strings.TrimSpace(v)); err == nil {
			return a.String()
		}
		return strings.TrimSpace(v)
	case "MX", "SRV":
		f := strings.Fields(v)
		if len(f) > 0 {
			f[len(f)-1] = host(f[len(f)-1])
		}
		return strings.ToLower(strings.Join(f, " "))
	}
	return host(v)
}

func wantKey(w Want) string {
	if w.Priority != nil {
		return Normalize(w.Type, fmt.Sprintf("%d %s", *w.Priority, w.Value))
	}
	return Normalize(w.Type, w.Value)
}

// txtOwner is the v= tag of a TXT value, so an SPF check ignores unrelated
// verification tokens at the same name.
func txtOwner(v string) string {
	l := Normalize("TXT", v)
	if !strings.HasPrefix(l, "v=") {
		return ""
	}
	if i := strings.IndexAny(l, "; "); i > 0 {
		return l[:i]
	}
	return l
}

// rivals reports whether an existing record of type have stops want from resolving.
func rivals(want, have string) bool {
	switch want {
	case "CNAME":
		return have == "A" || have == "AAAA"
	case "A", "AAAA", "MX", "TXT":
		return have == "CNAME"
	}
	return false
}

func base(w Want) Record {
	return Record{ID: w.ID, Type: w.Type, Name: w.Name, Value: w.Value, TTL: w.TTL, Priority: w.Priority, Found: []Found{}}
}

// classify grades one want against every record found at its name.
func classify(w Want, at []Found) Record {
	rec := base(w)
	owner := ""
	if w.Type == "TXT" {
		owner = txtOwner(w.Value)
	}
	var same, rival []Found
	for _, f := range at {
		switch {
		case f.Type == w.Type:
			if owner != "" && txtOwner(f.Value) != owner {
				continue
			}
			same = append(same, f)
		case rivals(w.Type, f.Type):
			rival = append(rival, f)
		}
	}
	rec.Found = append(append(rec.Found, same...), rival...)
	want := wantKey(w)
	for _, f := range same {
		if Normalize(w.Type, f.Value) == want {
			rec.Status = Live
			return rec
		}
	}
	if len(rec.Found) == 0 {
		rec.Status = Waiting
		return rec
	}
	rec.Status = Different
	rec.Reason = differentReason(w, same, rival)
	return rec
}

func differentReason(w Want, same, rival []Found) string {
	if len(rival) > 0 {
		return fmt.Sprintf("%s already has a %s record (%s), which stops a %s record from resolving.", w.Name, rival[0].Type, rival[0].Value, w.Type)
	}
	vals := make([]string, 0, len(same))
	for _, f := range same {
		vals = append(vals, f.Value)
		if w.Type == "MX" && w.Priority != nil {
			if p, h, ok := strings.Cut(Normalize("MX", f.Value), " "); ok && h == host(w.Value) && p != fmt.Sprint(*w.Priority) {
				return fmt.Sprintf("MX to %s exists with priority %s instead of %d.", h, p, *w.Priority)
			}
		}
	}
	return fmt.Sprintf("%s %s holds another value: %s.", w.Name, w.Type, strings.Join(vals, " | "))
}

// ZoneOptions tunes FromZone.
type ZoneOptions struct {
	// Shadow lists names mail must reach directly; a proxied record there is a conflict.
	Shadow map[string]bool
	// ProxyAware is true when the provider has a proxy flag, so false is meaningful.
	ProxyAware bool
}

// ZoneSet is one record set of a provider listing, with an absolute name.
type ZoneSet struct {
	Name    string
	Type    string
	TTL     int
	Values  []string
	Proxied bool
}

// FromZone grades wants against a provider's authoritative zone listing.
func FromZone(wants []Want, sets []ZoneSet, o ZoneOptions, now time.Time) Status {
	idx := map[string][]Found{}
	for _, s := range sets {
		name := host(s.Name)
		for _, v := range s.Values {
			f := Found{Type: s.Type, Name: name, Value: v, TTL: s.TTL}
			if s.Type == "TXT" {
				f.Value = JoinTXT(v)
			}
			if o.ProxyAware || s.Proxied {
				p := s.Proxied
				f.Proxied = &p
			}
			idx[name] = append(idx[name], f)
		}
	}
	recs := make([]Record, 0, len(wants))
	for _, w := range wants {
		rec := classify(w, idx[w.Name])
		for _, n := range reachNames(w) {
			if !o.Shadow[n] {
				continue
			}
			for _, f := range idx[n] {
				if f.Proxied == nil || !*f.Proxied || (f.Type != "A" && f.Type != "AAAA" && f.Type != "CNAME") {
					continue
				}
				rec.Status = Conflict
				rec.Reason = fmt.Sprintf("%s %s is proxied (orange cloud), so mail cannot reach it. Switch it to DNS only at the DNS provider, then check again.", n, f.Type)
				if !slices.Contains(rec.Found, f) {
					rec.Found = append(rec.Found, f)
				}
			}
		}
		recs = append(recs, rec)
	}
	return newStatus(SourceProvider, recs, now)
}

func reachNames(w Want) []string {
	out := slices.Clone(w.Reach)
	switch w.Type {
	case "A", "AAAA", "CNAME":
		out = append(out, w.Name)
	case "MX", "SRV":
		if f := strings.Fields(w.Value); len(f) > 0 {
			out = append(out, host(f[len(f)-1]))
		}
	}
	return out
}
