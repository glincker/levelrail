package mailrecords

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dnszones"
)

// Plan actions.
const (
	ActionAdd       = "add"
	ActionUnchanged = "unchanged"
	ActionReplace   = "replace"
	ActionRefused   = "refused"
)

// Errors Apply returns so callers can map them to a status.
var (
	ErrStalePlan = errors.New("the zone or the request changed since the plan was made; run plan again")
	ErrBlocked   = errors.New("the plan is blocked; fix the refusals or list the record types in replace_existing")
)

// PlanOptions controls what a plan may overwrite. The default is add-only.
type PlanOptions struct {
	ReplaceExisting []string `json:"replace_existing,omitempty"`
}

// Change is one record set in a plan. Set is the full desired content.
type Change struct {
	Action  string              `json:"action"`
	Kind    string              `json:"kind"`
	Set     dnszones.RecordSet  `json:"set"`
	Before  *dnszones.RecordSet `json:"before,omitempty"`
	Removed []string            `json:"removed,omitempty"`
	Reason  string              `json:"reason,omitempty"`
}

// PlanResult is the dry-run diff. Hash pins it so a stale plan cannot be applied.
type PlanResult struct {
	Domain    string         `json:"domain"`
	Provider  string         `json:"provider,omitempty"`
	Hash      string         `json:"hash"`
	Changes   []Change       `json:"changes"`
	Notes     []Note         `json:"notes,omitempty"`
	Refusals  []string       `json:"refusals,omitempty"`
	Replacing []string       `json:"replacing,omitempty"`
	Summary   map[string]int `json:"summary"`
	Blocked   bool           `json:"blocked"`
}

// NormalizeReplace validates and canonicalizes the record kinds a caller may replace.
func NormalizeReplace(in []string) ([]string, error) {
	var out []string
	for _, k := range in {
		k = strings.ToUpper(strings.TrimSpace(k))
		if k == "" {
			continue
		}
		if !slices.Contains(kindOrder, k) {
			return nil, fmt.Errorf("replace_existing %q is not one of %s", k, strings.Join(kindOrder, ", "))
		}
		if !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out, nil
}

// ownedValue reports whether an existing value belongs to the record kind, so a
// shared TXT set (apex SPF next to verification tokens) is edited, not replaced.
func ownedValue(kind, typ, v string) bool {
	if typ != "TXT" {
		return true
	}
	l := strings.ToLower(strings.TrimSpace(v))
	switch kind {
	case KindSPF:
		return strings.HasPrefix(l, "v=spf1")
	case KindDMARC:
		return strings.HasPrefix(l, "v=dmarc1")
	case KindMTASTS:
		return strings.HasPrefix(l, "v=stsv1")
	case KindTLSRPT:
		return strings.HasPrefix(l, "v=tlsrptv1")
	}
	return true
}

func sameValue(a, b string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " "))
}

// Plan reads the zone and diffs the desired records against it without writing.
func Plan(ctx context.Context, p dnszones.Provider, zone dnszones.Zone, spec Spec, opts PlanOptions) (PlanResult, error) {
	if dnszones.NormalizeDomain(zone.Name) != spec.Domain {
		return PlanResult{}, fmt.Errorf("zone %q does not match domain %q", zone.Name, spec.Domain)
	}
	replace, err := NormalizeReplace(opts.ReplaceExisting)
	if err != nil {
		return PlanResult{}, err
	}
	desired, notes, err := Build(spec)
	if err != nil {
		return PlanResult{}, err
	}
	existing, err := p.ListRecordSets(ctx, zone)
	if err != nil {
		return PlanResult{}, fmt.Errorf("list records: %w", err)
	}
	res := PlanResult{Domain: spec.Domain, Provider: p.Name(), Notes: notes, Replacing: replace, Summary: map[string]int{}}
	var writes []dnszones.RecordSet
	for _, d := range desired {
		ch := diffOne(d, existing, replace)
		if ch.Action == ActionRefused {
			res.Refusals = append(res.Refusals, ch.Reason)
		}
		if ch.Action == ActionAdd || ch.Action == ActionReplace {
			writes = append(writes, ch.Set)
		}
		res.Changes = append(res.Changes, ch)
	}
	for _, c := range dnszones.BuildPlan(existing, writes, false, p.Capabilities()).Changes {
		for _, is := range c.Issues {
			if is.Severity == dnszones.SeverityError {
				res.Refusals = append(res.Refusals, fmt.Sprintf("%s %s: %s", c.Set.Name, c.Set.Type, is.Message))
			}
		}
	}
	for _, ch := range res.Changes {
		res.Summary[ch.Action]++
	}
	res.Blocked = len(res.Refusals) > 0
	res.Hash = planHash(res)
	return res, nil
}

func diffOne(d Record, existing []dnszones.RecordSet, replace []string) Change {
	ch := Change{Action: ActionAdd, Kind: d.Kind, Set: d.Set}
	i := slices.IndexFunc(existing, func(e dnszones.RecordSet) bool { return e.Key() == d.Set.Key() })
	if i < 0 {
		return ch
	}
	before := existing[i]
	ch.Before = &before
	var owned, others []string
	for _, v := range before.Values {
		if ownedValue(d.Kind, d.Set.Type, v) {
			owned = append(owned, v)
		} else {
			others = append(others, v)
		}
	}
	if d.Set.Type == "TXT" {
		if len(owned) == 0 {
			ch.Set.Values = append(slices.Clone(others), d.Set.Values...)
			return ch
		}
		if len(owned) == len(d.Set.Values) && sameValue(owned[0], d.Set.Values[0]) {
			ch.Action = ActionUnchanged
			return ch
		}
	} else if allPresent(d.Set.Values, owned) {
		ch.Action = ActionUnchanged
		return ch
	}
	for _, v := range owned {
		if !slices.ContainsFunc(d.Set.Values, func(x string) bool { return sameValue(x, v) }) {
			ch.Removed = append(ch.Removed, v)
		}
	}
	if d.Set.Type == "TXT" {
		ch.Set.Values = append(slices.Clone(others), d.Set.Values...)
	}
	where := fmt.Sprintf("%s %s", d.Set.Name, d.Set.Type)
	if slices.Contains(replace, d.Kind) {
		ch.Action = ActionReplace
		ch.Reason = fmt.Sprintf("replaces %s", strings.Join(ch.Removed, " | "))
		return ch
	}
	ch.Action = ActionRefused
	ch.Reason = fmt.Sprintf("%s already has %s; add-only mode will not overwrite it. Add %q to replace_existing to replace it.", where, strings.Join(ch.Removed, " | "), d.Kind)
	if d.Kind == KindMX {
		ch.Reason = fmt.Sprintf("the domain already has MX records pointing elsewhere (%s); changing them reroutes live mail. Add MX to replace_existing only when you mean to move mail.", strings.Join(ch.Removed, " | "))
	}
	return ch
}

func allPresent(want, have []string) bool {
	for _, w := range want {
		if !slices.ContainsFunc(have, func(h string) bool { return sameValue(w, h) }) {
			return false
		}
	}
	return true
}

func planHash(r PlanResult) string {
	type pin struct {
		Action, Kind, Name, Type string
		Values, Before, Removed  []string
	}
	pins := make([]pin, 0, len(r.Changes))
	for _, c := range r.Changes {
		p := pin{c.Action, c.Kind, c.Set.Name, c.Set.Type, c.Set.Values, nil, c.Removed}
		if c.Before != nil {
			p.Before = c.Before.Values
		}
		pins = append(pins, p)
	}
	raw, _ := json.Marshal(struct {
		Domain  string
		Replace []string
		Pins    []pin
	}{r.Domain, r.Replacing, pins})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:16])
}

// Apply recomputes the plan, refuses it when the hash differs from the one the
// caller confirmed, and writes the add and replace changes.
func Apply(ctx context.Context, p dnszones.Provider, zone dnszones.Zone, spec Spec, opts PlanOptions, hash string) (int, PlanResult, error) {
	res, err := Plan(ctx, p, zone, spec, opts)
	if err != nil {
		return 0, res, err
	}
	if hash == "" || res.Hash != hash {
		return 0, res, ErrStalePlan
	}
	if res.Blocked {
		return 0, res, ErrBlocked
	}
	var writes []dnszones.RecordSet
	for _, c := range res.Changes {
		if c.Action == ActionAdd || c.Action == ActionReplace {
			writes = append(writes, c.Set)
		}
	}
	existing, err := p.ListRecordSets(ctx, zone)
	if err != nil {
		return 0, res, fmt.Errorf("list records: %w", err)
	}
	n, err := dnszones.ApplyPlan(ctx, p, zone, dnszones.BuildPlan(existing, writes, false, p.Capabilities()))
	return n, res, err
}

// AuditSubject lists record names and types only, never values.
func (r PlanResult) AuditSubject(applied int) string {
	var parts []string
	for _, c := range r.Changes {
		if c.Action == ActionAdd || c.Action == ActionReplace {
			parts = append(parts, c.Action+":"+c.Set.Name+"/"+c.Set.Type)
		}
	}
	return fmt.Sprintf("mail_dns applied=%d %s", applied, strings.Join(parts, ","))
}
