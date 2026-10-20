package dnszones

import (
	"context"
	"fmt"
	"slices"
)

// Plan actions.
const (
	ActionCreate    = "create"
	ActionUpdate    = "update"
	ActionDelete    = "delete"
	ActionUnchanged = "unchanged"
)

// Change is one planned record set change.
type Change struct {
	Action string     `json:"action"`
	Set    RecordSet  `json:"set"`
	Before *RecordSet `json:"before,omitempty"`
	Issues []Issue    `json:"issues,omitempty"`
}

// Plan is a diff between a zone and a desired set of records, shown before apply.
type Plan struct {
	Changes []Change       `json:"changes"`
	Summary map[string]int `json:"summary"`
	Blocked bool           `json:"blocked"`
}

// SameContent reports whether a and b would serve identical answers.
func SameContent(a, b RecordSet) bool {
	av, bv := slices.Clone(a.Values), slices.Clone(b.Values)
	slices.Sort(av)
	slices.Sort(bv)
	return a.TTL == b.TTL && a.Proxied == b.Proxied && slices.Equal(av, bv) &&
		a.HealthCheckID == b.HealthCheckID && a.Failover == b.Failover &&
		ptrEq(a.Weight, b.Weight)
}

func ptrEq(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// BuildPlan diffs desired against existing. With replace, sets in existing
// but not in desired are deleted, except provider managed apex NS and SOA.
// Without replace nothing is ever deleted, so a foreign record is never lost.
func BuildPlan(existing, desired []RecordSet, replace bool, caps Capabilities) Plan {
	byKey := make(map[Key]RecordSet, len(existing))
	for _, e := range existing {
		byKey[e.Key()] = e
	}
	var final []RecordSet
	plan := Plan{Summary: map[string]int{}}

	if replace {
		for _, e := range existing {
			if IsManagedApexType(e) || e.Alias != nil {
				final = append(final, e)
				continue
			}
			if !slices.ContainsFunc(desired, func(d RecordSet) bool { return d.Key() == e.Key() }) {
				plan.Changes = append(plan.Changes, Change{Action: ActionDelete, Set: e})
			}
		}
	}
	if replace {
		final = append(final, desired...)
	} else {
		final = mergeDesired(existing, desired)
	}
	for _, d := range desired {
		ch := Change{Action: ActionCreate, Set: d}
		k := d.Key()
		if before, ok := byKey[k]; ok {
			b := before
			ch.Before = &b
			ch.Action = ActionUpdate
			if SameContent(before, d) {
				ch.Action = ActionUnchanged
			}
		}
		if err := CheckCapabilities(d, caps); err != nil {
			ch.Issues = append(ch.Issues, Issue{SeverityError, "unsupported", err.Error()})
		}
		for _, is := range Conflicts(final, d, caps, &k) {
			if is.Code != IssueExists {
				ch.Issues = append(ch.Issues, is)
			}
		}
		plan.Changes = append(plan.Changes, ch)
	}
	for _, ch := range plan.Changes {
		plan.Summary[ch.Action]++
		if HasErrors(ch.Issues) {
			plan.Blocked = true
		}
	}
	return plan
}

// mergeDesired returns existing with desired sets layered over it by key.
func mergeDesired(existing, desired []RecordSet) []RecordSet {
	out := make([]RecordSet, 0, len(existing)+len(desired))
	for _, e := range existing {
		if !slices.ContainsFunc(desired, func(d RecordSet) bool { return d.Key() == e.Key() }) {
			out = append(out, e)
		}
	}
	return append(out, desired...)
}

// ApplyPlan executes a plan: deletes first (so a CNAME can replace an A),
// then updates and creates. It refuses a blocked plan.
func ApplyPlan(ctx context.Context, p Provider, zone Zone, plan Plan) (int, error) {
	if plan.Blocked {
		return 0, fmt.Errorf("dnszones: plan has conflicts; fix them before applying")
	}
	applied := 0
	for _, pass := range []string{ActionDelete, ActionUpdate, ActionCreate} {
		for _, ch := range plan.Changes {
			if ch.Action != pass {
				continue
			}
			var err error
			if pass == ActionDelete {
				err = p.DeleteRecordSet(ctx, zone, ch.Set.Key())
			} else {
				err = p.UpsertRecordSet(ctx, zone, ch.Set)
			}
			if err != nil {
				return applied, fmt.Errorf("dnszones: %s %s %s: %w", pass, ch.Set.Name, ch.Set.Type, err)
			}
			applied++
		}
	}
	return applied, nil
}
