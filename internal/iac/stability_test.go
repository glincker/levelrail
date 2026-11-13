package iac

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestKindStability(t *testing.T) {
	for _, k := range []Kind{KindLoadBalancer, KindPipeline, KindAlertRule} {
		if KindStability(k) != StabilityBeta {
			t.Errorf("%s should be beta", k)
		}
		if betaWarning(k) == "" {
			t.Errorf("%s has no beta warning", k)
		}
	}
	for _, k := range Kinds {
		if KindStability(k) == "" {
			t.Errorf("%s has no stability", k)
		}
	}
	if KindStability(KindApp) != StabilityStable || betaWarning(KindApp) != "" {
		t.Error("App should be stable with no warning")
	}
	// Every kind in the enum must be classified in the schema.
	var own struct {
		Stability map[string]string `json:"x-stability"`
	}
	if err := json.Unmarshal(resourcesSchema, &own); err != nil {
		t.Fatal(err)
	}
	for _, k := range Kinds {
		if _, ok := own.Stability[string(k)]; !ok {
			t.Errorf("%s missing from x-stability", k)
		}
	}
}

func TestKindStabilityWarningInPlan(t *testing.T) {
	docs, issues := ParseDocuments([]Source{{Name: "a.yaml", Data: []byte("version: 1\nkind: AlertRule\nmetadata:\n  name: r\nspec: {app: web, type: threshold, metric: cpu_percent, comparator: \">\", threshold: 90, for: 5m, channel: ops}\n")}})
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	res, bi := Build(docs, Options{})
	if len(bi) > 0 {
		t.Fatalf("fixture invalid: %v", bi)
	}
	if len(res) != 1 || !strings.Contains(strings.Join(res[0].Warnings, " "), "beta") {
		t.Fatalf("expected beta warning, got %+v", res)
	}
}

// Pins that a load balancer document fails with the unavailable code when
// the control plane answers 501 for the feature, rather than being dropped.
func TestKindStabilityLoadBalancerFlagOff(t *testing.T) {
	st := &State{denied: map[string]error{}}
	err := st.note(resourceKey(KindLoadBalancer, "", "web"), &StatusError{Status: http.StatusNotImplemented, Message: "off"})
	if err != nil {
		t.Fatal(err)
	}
	r := &Resource{Kind: KindLoadBalancer, Name: "web", Fields: map[string]any{}}
	p := &planner{st: st, byKey: map[string]*Resource{}}
	p.planLB(r)
	if len(p.items) != 1 || p.items[0].Code != CodeUnavailable {
		t.Fatalf("expected unavailable item, got %+v", p.items)
	}
}
