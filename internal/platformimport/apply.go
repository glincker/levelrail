package platformimport

import (
	"context"
	"fmt"
)

// Applier creates resources on this platform. Implementations must be
// safe to call again after a partial failure.
type Applier interface {
	CreateApp(ctx context.Context, p AppPlan) (warnings []string, err error)
	CreateDatabase(ctx context.Context, p DatabasePlan) (warnings []string, err error)
}

// Apply creates every planned resource, continuing past individual
// failures so a re-run (which skips what already exists) can finish the
// rest. It returns the plan's report with created and failed statuses.
func Apply(ctx context.Context, plan *Plan, ap Applier) Report {
	rep := plan.Report
	rep.Items = append([]Item(nil), plan.Report.Items...)
	index := func(kind, id string) int {
		for i, it := range rep.Items {
			if it.Kind == kind && it.SourceID == id {
				return i
			}
		}
		return -1
	}
	mark := func(kind, id string, warnings []string, err error) {
		i := index(kind, id)
		if i < 0 {
			return
		}
		rep.Items[i].Reasons = append(rep.Items[i].Reasons, warnings...)
		if len(warnings) > 0 && rep.Items[i].Status == StatusMapped {
			rep.Items[i].Status = StatusNeedsAttention
		}
		if err != nil {
			rep.Items[i].Status = StatusFailed
			rep.Items[i].Reasons = append(rep.Items[i].Reasons, "create failed: "+err.Error())
			return
		}
		rep.Items[i].Reasons = append(rep.Items[i].Reasons, "created")
		rep.Items[i].Status = StatusCreated
	}
	for _, a := range plan.Apps {
		if err := ctx.Err(); err != nil {
			mark("app", a.SourceID, nil, fmt.Errorf("stopped: %w", err))
			continue
		}
		w, err := ap.CreateApp(ctx, a)
		mark("app", a.SourceID, w, err)
	}
	for _, d := range plan.Databases {
		if err := ctx.Err(); err != nil {
			mark("database", d.SourceID, nil, fmt.Errorf("stopped: %w", err))
			continue
		}
		w, err := ap.CreateDatabase(ctx, d)
		mark("database", d.SourceID, w, err)
	}
	rep.Counts = map[string]int{}
	for _, it := range rep.Items {
		rep.Counts[it.Status]++
	}
	return rep
}
