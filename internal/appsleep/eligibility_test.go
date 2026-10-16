package appsleep

import (
	"net/http"
	"testing"
)

func codes(items []Item) map[string]bool {
	m := map[string]bool{}
	for _, i := range items {
		m[i.Code] = true
	}
	return m
}

func TestEvaluateMatrix(t *testing.T) {
	pinned := []int{3000}
	tests := []struct {
		name         string
		f            Facts
		wantBlockers []string
		wantWarnings []string
		notWarnings  []string
		wantFix      string
	}{
		{name: "plain app with domain", f: Facts{App: "web", HasDomain: true, Replicas: 1}, wantWarnings: []string{CodeBackgroundWork}, notWarnings: []string{CodeNoDomain, CodeDirectTraffic}},
		{name: "pinned port, port wake on", f: Facts{App: "web", PinnedPorts: pinned, PortWakeSupported: true, PortWakeEnabled: true, HasDomain: true}, wantWarnings: []string{CodeDirectTraffic}},
		{name: "pinned port, port wake off", f: Facts{App: "web", PinnedPorts: pinned, PortWakeSupported: true}, wantBlockers: []string{CodePinnedPort}, wantFix: FixEnablePortWake},
		{name: "pinned port, port wake unavailable", f: Facts{App: "web", PinnedPorts: pinned, PortWakeUnsupportedReason: "the app runs on another server"}, wantBlockers: []string{CodePinnedPort}, wantFix: FixRemovePinnedPort},
		{name: "pinned port held by another process", f: Facts{App: "web", PinnedPorts: pinned, PortWakeSupported: true, PortWakeEnabled: true, PortWakeError: "address already in use"}, wantBlockers: []string{CodePortInUse}, wantFix: FixRemovePinnedPort},
		{name: "managed database", f: Facts{App: "db", IsManagedDatabase: true, HasDomain: true}, wantBlockers: []string{CodeDatabase}},
		{name: "node cordoned", f: Facts{App: "web", NodeUnschedulable: true, HasDomain: true}, wantBlockers: []string{CodeNodeUnschedulable}},
		{name: "canary target", f: Facts{App: "web", CanaryTarget: true, HasDomain: true}, wantBlockers: []string{CodeCanaryTarget}},
		{name: "dependency without hold is only a warning", f: Facts{App: "api", DependedOnBy: []string{"web"}, HasDomain: true}, wantWarnings: []string{CodeDependency}, wantFix: FixSetHoldRequests},
		{name: "dependency with hold has no warning", f: Facts{App: "api", DependedOnBy: []string{"web"}, HoldRequests: true, HasDomain: true}, notWarnings: []string{CodeDependency}},
		{name: "no domain", f: Facts{App: "web"}, wantWarnings: []string{CodeNoDomain}, wantFix: FixAttachDomain},
		{name: "tcp stream", f: Facts{App: "web", HasStreams: true, HasDomain: true}, wantWarnings: []string{CodeDirectTraffic}},
		{name: "trial and replicas", f: Facts{App: "web", IsTrial: true, Replicas: 3, HasDomain: true}, wantWarnings: []string{CodeTrial, CodeReplicas}},
		{name: "database image", f: Facts{App: "pg", StatefulImage: true, HasDomain: true}, wantWarnings: []string{CodeStatefulImage}},
		{name: "last wake failed", f: Facts{App: "web", LastWakeError: "not ready", HasDomain: true}, wantWarnings: []string{CodeLastWakeFailed}},
		{name: "several blockers at once", f: Facts{App: "db", IsManagedDatabase: true, CanaryTarget: true, NodeUnschedulable: true}, wantBlockers: []string{CodeDatabase, CodeCanaryTarget, CodeNodeUnschedulable}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := Evaluate(tc.f)
			gotB, gotW := codes(e.Blockers), codes(e.Warnings)
			if len(gotB) != len(tc.wantBlockers) {
				t.Errorf("blockers = %v, want %v", gotB, tc.wantBlockers)
			}
			for _, c := range tc.wantBlockers {
				if !gotB[c] {
					t.Errorf("missing blocker %s in %v", c, gotB)
				}
			}
			for _, c := range tc.wantWarnings {
				if !gotW[c] {
					t.Errorf("missing warning %s in %v", c, gotW)
				}
			}
			for _, c := range tc.notWarnings {
				if gotW[c] {
					t.Errorf("unexpected warning %s", c)
				}
			}
			if e.Eligible() != (len(tc.wantBlockers) == 0) {
				t.Errorf("Eligible() = %v", e.Eligible())
			}
			if tc.wantFix != "" {
				found := false
				for _, it := range append(append([]Item{}, e.Blockers...), e.Warnings...) {
					if it.Fix != nil && it.Fix.Kind == tc.wantFix {
						found = true
					}
				}
				if !found {
					t.Errorf("no fix of kind %s", tc.wantFix)
				}
			}
		})
	}
}

func TestFixesCarryTheRightCall(t *testing.T) {
	e := Evaluate(Facts{App: "web", IdleMinutes: 45, PinnedPorts: []int{3000}, PortWakeSupported: true})
	f := e.Blockers[0].Fix
	if f.Method != http.MethodPut || f.Path != "/api/v1/apps/web/sleep" || f.Body["port_wake"] != true || f.Body["idle_minutes"] != 45 {
		t.Errorf("enable_port_wake fix = %+v", f)
	}
	e = Evaluate(Facts{App: "web", PinnedPorts: []int{3000}})
	f = e.Blockers[0].Fix
	if f.Method != http.MethodPut || f.Path != "/api/v1/apps/web/exposure" || f.Body["mode"] != "proxied" {
		t.Errorf("remove_pinned_port fix = %+v", f)
	}
}
