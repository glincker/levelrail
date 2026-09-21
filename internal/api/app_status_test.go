package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
)

func TestSummarizeAppConditions(t *testing.T) {
	tests := []struct {
		name       string
		conditions []reconcile.Condition
		want       appStatusSummary
	}{
		{
			name:       "no conditions",
			conditions: nil,
			want:       appStatusSummary{Label: "No status yet", Variant: "muted"},
		},
		{
			name: "all true is healthy",
			conditions: []reconcile.Condition{
				{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Deployed"},
			},
			want: appStatusSummary{Label: "Healthy", Variant: "success"},
		},
		{
			name: "a false condition needs attention regardless of others",
			conditions: []reconcile.Condition{
				{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Deployed"},
				{Type: "EgressPolicyReady", Status: reconcile.ConditionFalse, Reason: "PolicyApplyFailed"},
			},
			want: appStatusSummary{Label: "Attention needed", Variant: "destructive"},
		},
		{
			name: "suspended overrides everything else",
			conditions: []reconcile.Condition{
				{Type: "Ready", Status: reconcile.ConditionUnknown, Reason: "Suspended"},
			},
			want: appStatusSummary{Label: "Stopped", Variant: "muted"},
		},
		{
			name: "an unconfigured optional feature does not block healthy",
			conditions: []reconcile.Condition{
				{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "AlreadyRunning"},
				{Type: "EgressPolicyReady", Status: reconcile.ConditionUnknown, Reason: "NotConfigured"},
			},
			want: appStatusSummary{Label: "Healthy", Variant: "success"},
		},
		{
			name: "a disabled optional integration does not block healthy",
			conditions: []reconcile.Condition{
				{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Deployed"},
				{Type: "Ready", Status: reconcile.ConditionUnknown, Reason: "Disabled"},
			},
			want: appStatusSummary{Label: "Healthy", Variant: "success"},
		},
		{
			name: "a genuinely pending unknown condition still reconciles",
			conditions: []reconcile.Condition{
				{Type: "Ready", Status: reconcile.ConditionUnknown, Reason: "AwaitingFirstBuild"},
			},
			want: appStatusSummary{Label: "Reconciling", Variant: "muted"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summarizeAppConditions(tt.conditions); got != tt.want {
				t.Errorf("summarizeAppConditions() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
