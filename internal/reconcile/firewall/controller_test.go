package firewall

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/firewall"
)

type fakeRuleStore struct {
	rules []store.FirewallRule
	err   error
}

func (f *fakeRuleStore) ListFirewallRules(context.Context) ([]store.FirewallRule, error) {
	return f.rules, f.err
}

type fakeSyncer struct {
	result firewall.Result
	err    error
	gotLen int
}

func (f *fakeSyncer) Sync(_ context.Context, want []firewall.Rule) (firewall.Result, error) {
	f.gotLen = len(want)
	return f.result, f.err
}

func readyCondition(result reconcile.Result) *reconcile.Condition {
	for _, c := range result.Conditions {
		if c.Type == reconcile.ConditionTypeReady {
			return &c
		}
	}
	return nil
}

func TestReconcile_SyncsLocalRulesOnly(t *testing.T) {
	ruleStore := &fakeRuleStore{rules: []store.FirewallRule{
		{ID: "a", NodeID: "", Port: 9000, Protocol: "tcp", Action: store.FirewallRuleActionAllow},
		{ID: "b", NodeID: "remote-node", Port: 9001, Protocol: "tcp", Action: store.FirewallRuleActionAllow},
	}}
	syncer := &fakeSyncer{result: firewall.Result{Installed: true, Active: true}}
	c := New(ruleStore, syncer)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if syncer.gotLen != 1 {
		t.Fatalf("expected the remote-node rule to be excluded from Sync's want list, got len=%d", syncer.gotLen)
	}
	cond := readyCondition(result)
	if cond == nil || cond.Status != reconcile.ConditionTrue {
		t.Fatalf("expected Ready=True, got %+v", result.Conditions)
	}
}

func TestReconcile_SkipsUnsafeRule(t *testing.T) {
	ruleStore := &fakeRuleStore{rules: []store.FirewallRule{
		{ID: "a", NodeID: "", Port: 9443, Protocol: "tcp", Action: store.FirewallRuleActionDeny},
	}}
	syncer := &fakeSyncer{result: firewall.Result{Installed: true, Active: true}}
	c := New(ruleStore, syncer, WithRequiredPorts([]int{9443}))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if syncer.gotLen != 0 {
		t.Fatalf("expected the unsafe deny-on-required-port rule to be excluded, got len=%d", syncer.gotLen)
	}
	cond := readyCondition(result)
	if cond == nil || cond.Status != reconcile.ConditionTrue {
		t.Fatalf("expected Ready=True even with a skipped unsafe rule, got %+v", result.Conditions)
	}
}

// Half-succeeded case: Sync itself reports a per-rule error (e.g. one
// ufw invocation failed) without returning a Go error; Reconcile must
// surface that as a False condition, not silently report success.
func TestReconcile_SyncPartialFailureReportsFalse(t *testing.T) {
	ruleStore := &fakeRuleStore{rules: []store.FirewallRule{
		{ID: "a", NodeID: "", Port: 9000, Protocol: "tcp", Action: store.FirewallRuleActionAllow},
	}}
	syncer := &fakeSyncer{result: firewall.Result{
		Installed: true, Active: true,
		Errors: []string{"allow 9000/tcp (rule:a): ufw: could not acquire lock"},
	}}
	c := New(ruleStore, syncer)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile should not return a Go error for a per-rule sync failure: %v", err)
	}
	cond := readyCondition(result)
	if cond == nil || cond.Status != reconcile.ConditionFalse || cond.Reason != "RuleSyncFailed" {
		t.Fatalf("expected Ready=False/RuleSyncFailed, got %+v", result.Conditions)
	}
}

func TestReconcile_StoreErrorPropagates(t *testing.T) {
	ruleStore := &fakeRuleStore{err: errors.New("db unavailable")}
	syncer := &fakeSyncer{}
	c := New(ruleStore, syncer)

	_, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("expected an error when the store fails")
	}
}

func TestReconcile_UFWNotInstalledIsUnknownNotFalse(t *testing.T) {
	ruleStore := &fakeRuleStore{}
	syncer := &fakeSyncer{result: firewall.Result{Installed: false}}
	c := New(ruleStore, syncer)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	cond := readyCondition(result)
	if cond == nil || cond.Status != reconcile.ConditionUnknown || cond.Reason != "UFWNotInstalled" {
		t.Fatalf("expected Ready=Unknown/UFWNotInstalled, got %+v", result.Conditions)
	}
}
