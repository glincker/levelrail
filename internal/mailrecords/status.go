package mailrecords

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dnsstatus"
	"github.com/GLINCKER/levelrail/internal/dnszones"
)

// Wants turns built records into one status entry per value, with ids stable
// by kind and type (mx-mx, spf-txt, host-a, mx-mx-2 for a second value).
func Wants(desired []Record, domain string) []dnsstatus.Want {
	domain = dnszones.NormalizeDomain(domain)
	seen := map[string]int{}
	out := make([]dnsstatus.Want, 0, len(desired))
	for _, d := range desired {
		for _, v := range d.Set.Values {
			id := strings.ToLower(d.Kind + "-" + d.Set.Type)
			seen[id]++
			if n := seen[id]; n > 1 {
				id = fmt.Sprintf("%s-%d", id, n)
			}
			w := dnsstatus.Want{ID: id, Type: d.Set.Type, Name: dnszones.FQDN(d.Set.Name, domain), Value: v, TTL: d.Set.TTL}
			switch d.Set.Type {
			case "MX", "SRV":
				if p, rest, ok := strings.Cut(strings.TrimSpace(v), " "); ok {
					if n, err := strconv.Atoi(p); err == nil {
						w.Priority, w.Value = &n, strings.TrimSpace(rest)
					}
				}
				w.Value = strings.TrimSuffix(w.Value, ".")
			case "TXT":
				w.Value = dnsstatus.JoinTXT(v)
			case "CNAME":
				w.Value = strings.TrimSuffix(v, ".")
			}
			if d.Kind == KindMTASTS {
				w.Reach = []string{"mta-sts." + domain}
			}
			out = append(out, w)
		}
	}
	return out
}

// ZoneSets converts a provider listing for dnsstatus.FromZone.
func ZoneSets(sets []dnszones.RecordSet, zone string) []dnsstatus.ZoneSet {
	out := make([]dnsstatus.ZoneSet, 0, len(sets))
	for _, s := range sets {
		out = append(out, dnsstatus.ZoneSet{Name: dnszones.FQDN(s.Name, zone), Type: s.Type, TTL: s.TTL, Values: s.Values, Proxied: s.Proxied})
	}
	return out
}

// ShadowNames are the names a proxied record would hide from mail.
func ShadowNames(desired []Record, domain string) map[string]bool {
	return mailHostNames(desired, dnszones.NormalizeDomain(domain))
}

// toAdd lists the status ids an apply would write: live records never count.
func toAdd(res PlanResult) []string {
	keys := map[string]bool{}
	for _, c := range res.Changes {
		if c.Action == ActionAdd || c.Action == ActionReplace {
			keys[dnszones.FQDN(c.Set.Name, res.Domain)+"|"+c.Set.Type] = true
		}
	}
	out := []string{}
	for _, r := range res.Records {
		if keys[r.Name+"|"+r.Type] && r.Status != dnsstatus.Live {
			out = append(out, r.ID)
		}
	}
	return out
}
