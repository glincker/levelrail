package preflight

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/policy"
)

func TestPolicyChecks(t *testing.T) {
	tests := []struct {
		name       string
		set        policy.Set
		req        Request
		wantStatus Status
		wantCode   string
	}{
		{"no rules", nil, Request{Image: "x/y"}, StatusPass, ""},
		{"enforce fails", policy.Set{policy.RuleForbidPrivileged: {Mode: policy.ModeEnforce}}, Request{Privileged: true}, StatusFail, policy.CodePrivilegedForbidden},
		{"warn warns", policy.Set{policy.RuleRequireReadiness: {Mode: policy.ModeWarn}}, Request{}, StatusWarn, policy.CodeReadinessRequired},
		{"clean passes", policy.Set{policy.RuleRequireReadiness: {Mode: policy.ModeEnforce}}, Request{HasReadiness: true}, StatusPass, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep := Run(context.Background(), tc.req, Env{Policy: tc.set})
			if rep.Status != tc.wantStatus {
				t.Fatalf("status = %s, want %s: %+v", rep.Status, tc.wantStatus, rep.Checks)
			}
			if tc.wantCode == "" {
				return
			}
			for _, c := range rep.Checks {
				if c.Code == tc.wantCode && c.Fix != "" {
					return
				}
			}
			t.Fatalf("no check with code %s and a fix: %+v", tc.wantCode, rep.Checks)
		})
	}
}
