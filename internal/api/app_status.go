package api

import "github.com/GLINCKER/levelrail/internal/reconcile"

// Mirrors web/src/lib/appStatus.ts summarizeAppStatus, so the labels and variants must be kept in sync.
type appStatusSummary struct {
	Label   string `json:"label"`
	Variant string `json:"variant"`
}

// Mirrors summarizeAppStatus: any False wins, then all True means healthy, otherwise still reconciling.
func summarizeAppConditions(conditions []reconcile.Condition) appStatusSummary {
	if len(conditions) == 0 {
		return appStatusSummary{Label: "No status yet", Variant: "muted"}
	}
	for _, c := range conditions {
		if c.Reason == "Suspended" {
			return appStatusSummary{Label: "Stopped", Variant: "muted"}
		}
	}
	allTrue := true
	for _, c := range conditions {
		if c.Status == reconcile.ConditionFalse {
			return appStatusSummary{Label: "Attention needed", Variant: "destructive"}
		}
		if c.Status != reconcile.ConditionTrue && !isOptionalFeatureUnconfigured(c) {
			allTrue = false
		}
	}
	if allTrue {
		return appStatusSummary{Label: "Healthy", Variant: "success"}
	}
	return appStatusSummary{Label: "Reconciling", Variant: "muted"}
}

// Without this, an unconfigured optional feature stays Unknown forever and the app never leaves Reconciling.
func isOptionalFeatureUnconfigured(c reconcile.Condition) bool {
	return c.Status == reconcile.ConditionUnknown && (c.Reason == "NotConfigured" || c.Reason == "Disabled")
}

// Batched status lives on the list endpoint only, so per-app endpoints avoid a second, drifting status representation.
type appListResource struct {
	appResource
	Status appStatusSummary `json:"status"`
	// EnvironmentName is the resolved name of appResource.EnvironmentID, empty when untagged.
	EnvironmentName string `json:"environment_name,omitempty"`
	// Sleep is set only for apps with sleep-when-idle on.
	Sleep *appListSleep `json:"sleep,omitempty"`
	// ManagedByStack names the stack owning this app, omitted when none.
	ManagedByStack string `json:"managed_by_stack,omitempty"`
}
