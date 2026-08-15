package api

import "github.com/GLINCKER/levelrail/internal/reconcile"

// appStatusSummary is a compact category summary of an app's current
// reconcile conditions, mirroring web/src/lib/appStatus.ts's
// summarizeAppStatus exactly (same label wording, same variant names):
// the same "define locally, keep semantics identical, document the
// mirror" convention applicationControllerName's own doc comment
// (deploys.go) already establishes for a cross-language duplication.
// Kept deliberately narrow (label + variant) rather than the raw
// conditions list: AppRow only needs enough to render a status dot, the
// full reconcile detail already has a home in the app detail page's
// ConditionsPanel (GET /api/v1/apps/{name}/deploys).
type appStatusSummary struct {
	Label   string `json:"label"`
	Variant string `json:"variant"`
}

// summarizeAppConditions mirrors web/src/lib/appStatus.ts's
// summarizeAppStatus category-by-category, in the same order: a single
// False condition means attention needed regardless of how many others
// are True, all-True means healthy, anything else (some Unknown, none
// False) means still reconciling.
func summarizeAppConditions(conditions []reconcile.Condition) appStatusSummary {
	if len(conditions) == 0 {
		return appStatusSummary{Label: "No status yet", Variant: "muted"}
	}
	allTrue := true
	for _, c := range conditions {
		if c.Status == reconcile.ConditionFalse {
			return appStatusSummary{Label: "Attention needed", Variant: "destructive"}
		}
		if c.Status != reconcile.ConditionTrue {
			allTrue = false
		}
	}
	if allTrue {
		return appStatusSummary{Label: "Healthy", Variant: "success"}
	}
	return appStatusSummary{Label: "Reconciling", Variant: "muted"}
}

// appListResource is GET /api/v1/apps' wire shape (apps.go's
// handleListApps): appResource plus a batched status summary. Scoped to
// the list endpoint only, not appResource itself, so create/get/update/
// setNode/setProject/restart stay exactly as they were: those already
// have a richer status surface available via GET
// /api/v1/apps/{name}/deploys (handleDeployHistory) that the frontend's
// ConditionsPanel/hero badge reads directly, and duplicating a second,
// easily-drifting status representation onto every one of those
// endpoints would serve no consumer that needs it. AppRow (the apps
// list row) is the one consumer this batches for: without it, rendering
// a status dot per row would mean an N+1 GET .../deploys call past 50+
// apps, exactly the gap AppRow.tsx's own doc comment used to describe.
type appListResource struct {
	appResource
	Status appStatusSummary `json:"status"`
}
