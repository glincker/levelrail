package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func allowlistPolicy(entries ...store.ServiceEgressAllow) *store.ServiceEgressPolicy {
	return &store.ServiceEgressPolicy{Mode: store.EgressModeAllowlist, Allow: entries}
}

func TestEgressAllowEnvValue(t *testing.T) {
	tests := []struct {
		name  string
		allow []store.ServiceEgressAllow
		want  string
	}{
		{name: "empty", allow: nil, want: ""},
		{name: "single", allow: []store.ServiceEgressAllow{{Host: "api.example.com", Port: 443}}, want: "api.example.com:443"},
		{
			name: "multiple preserve order",
			allow: []store.ServiceEgressAllow{
				{Host: "api.anthropic.com", Port: 443},
				{Host: "github.com", Port: 443},
			},
			want: "api.anthropic.com:443 github.com:443",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := egressAllowEnvValue(tt.allow); got != tt.want {
				t.Errorf("egressAllowEnvValue(%+v) = %q, want %q", tt.allow, got, tt.want)
			}
		})
	}
}

func TestIsEgressSidecarName(t *testing.T) {
	target := ContainerName("web", "img:v1", "")
	sidecar := egressSidecarName(target)

	got, ok := isEgressSidecarName("web", sidecar)
	if !ok || got != target {
		t.Errorf("isEgressSidecarName(%q, %q) = (%q, %v), want (%q, true)", "web", sidecar, got, ok, target)
	}

	if _, ok := isEgressSidecarName("web", target); ok {
		t.Errorf("isEgressSidecarName(%q, %q) = ok, want not-ok: the app container's own name is not a sidecar name", "web", target)
	}
	if _, ok := isEgressSidecarName("worker", sidecar); ok {
		t.Errorf("isEgressSidecarName(%q, %q) = ok, want not-ok: sidecar belongs to a different service", "worker", sidecar)
	}
}

func TestReconcileEgress_NotConfigured_NoOp(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)
	c := New("web", &fakeStore{svc: desired}, rt)

	cond := c.reconcileEgress(context.Background(), []string{target}, desired)

	if cond.Type != "EgressPolicyReady" || cond.Status != reconcile.ConditionUnknown || cond.Reason != "NotConfigured" {
		t.Errorf("condition = %+v, want Type=EgressPolicyReady Status=Unknown Reason=NotConfigured", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0: no sidecar should ever be created when egress isn't configured", rt.createCalls)
	}
}

func TestReconcileEgress_AwaitingAppContainer(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Egress: allowlistPolicy(store.ServiceEgressAllow{Host: "api.example.com", Port: 443})}
	target := ContainerName("web", desired.Image, "")
	// Deliberately not seeded: the app container doesn't exist yet.
	c := New("web", &fakeStore{svc: desired}, rt)

	cond := c.reconcileEgress(context.Background(), []string{target}, desired)

	if cond.Status != reconcile.ConditionFalse || cond.Reason != "PolicyApplyFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=PolicyApplyFailed", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0: a sidecar can't attach to a netns that doesn't exist yet", rt.createCalls)
	}
}

func TestReconcileEgress_CreatesSidecarAttachedToAppContainer(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Egress: allowlistPolicy(store.ServiceEgressAllow{Host: "api.example.com", Port: 443}),
	}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)
	appState, _ := rt.InspectByName(context.Background(), target)
	c := New("web", &fakeStore{svc: desired}, rt)

	cond := c.reconcileEgress(context.Background(), []string{target}, desired)

	if cond.Status != reconcile.ConditionTrue || cond.Reason != "PoliciesApplied" {
		t.Fatalf("condition = %+v, want Status=True Reason=PoliciesApplied", cond)
	}
	if rt.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", rt.createCalls)
	}

	spec := rt.lastCreateSpec
	wantName := egressSidecarName(target)
	if spec.Name != wantName {
		t.Errorf("sidecar Name = %q, want %q", spec.Name, wantName)
	}
	if spec.Image != egressImage {
		t.Errorf("sidecar Image = %q, want %q", spec.Image, egressImage)
	}
	if len(spec.CapAdd) != 1 || spec.CapAdd[0] != "NET_ADMIN" {
		t.Errorf("sidecar CapAdd = %v, want [NET_ADMIN]", spec.CapAdd)
	}
	wantNetworkMode := "container:" + appState.ID
	if spec.NetworkMode != wantNetworkMode {
		t.Errorf("sidecar NetworkMode = %q, want %q", spec.NetworkMode, wantNetworkMode)
	}
	if got := spec.Env[egressAllowEnv]; got != "api.example.com:443" {
		t.Errorf("sidecar Env[%s] = %q, want %q", egressAllowEnv, got, "api.example.com:443")
	}
	if got := spec.Labels[egressTargetIDLabelKey]; got != appState.ID {
		t.Errorf("sidecar Labels[%s] = %q, want %q", egressTargetIDLabelKey, got, appState.ID)
	}
}

func TestReconcileEgress_AlreadyApplied_NoOp(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Egress: allowlistPolicy(store.ServiceEgressAllow{Host: "api.example.com", Port: 443})}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)
	appState, _ := rt.InspectByName(context.Background(), target)
	rt.seedLabeled(egressSidecarName(target), true, map[string]string{egressTargetIDLabelKey: appState.ID})
	c := New("web", &fakeStore{svc: desired}, rt)

	cond := c.reconcileEgress(context.Background(), []string{target}, desired)

	if cond.Status != reconcile.ConditionTrue || cond.Reason != "PoliciesApplied" {
		t.Errorf("condition = %+v, want Status=True Reason=PoliciesApplied", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0: an already-correct sidecar must not be recreated", rt.createCalls)
	}
	if rt.removeCalls != 0 {
		t.Errorf("removeCalls = %d, want 0", rt.removeCalls)
	}
}

// TestReconcileEgress_StaleAppContainerID_Recreates proves the
// egressTargetIDLabelKey check: a sidecar whose label no longer matches
// the app container's current ID (an operator manually removed and
// Levelrail self-healed the app container under the same deterministic
// name) is stale and must be recreated, not treated as already applied.
func TestReconcileEgress_StaleAppContainerID_Recreates(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Egress: allowlistPolicy(store.ServiceEgressAllow{Host: "api.example.com", Port: 443})}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)
	appState, _ := rt.InspectByName(context.Background(), target)
	rt.seedLabeled(egressSidecarName(target), true, map[string]string{egressTargetIDLabelKey: "stale-id-from-a-removed-container"})
	c := New("web", &fakeStore{svc: desired}, rt)

	cond := c.reconcileEgress(context.Background(), []string{target}, desired)

	if cond.Status != reconcile.ConditionTrue || cond.Reason != "PoliciesApplied" {
		t.Fatalf("condition = %+v, want Status=True Reason=PoliciesApplied", cond)
	}
	if rt.removeCalls != 1 {
		t.Errorf("removeCalls = %d, want 1 (the stale sidecar)", rt.removeCalls)
	}
	if rt.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1 (the replacement)", rt.createCalls)
	}
	if got := rt.lastCreateSpec.Labels[egressTargetIDLabelKey]; got != appState.ID {
		t.Errorf("replacement sidecar Labels[%s] = %q, want current app container ID %q", egressTargetIDLabelKey, got, appState.ID)
	}
}

// TestReconcileEgress_OrphanedSidecarRemovedOnTargetChange proves an
// image-change-driven target rename (a fresh blue-green deploy's new
// container name) leaves the old sidecar as an orphan, and this cleans
// it up rather than leaking it forever.
func TestReconcileEgress_OrphanedSidecarRemovedOnTargetChange(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Egress: allowlistPolicy(store.ServiceEgressAllow{Host: "api.example.com", Port: 443})}
	oldTarget := ContainerName("web", "img:v1", "")
	rt.seedLabeled(egressSidecarName(oldTarget), true, map[string]string{egressTargetIDLabelKey: "old-app-id"})
	newTarget := ContainerName("web", "img:v2", "")
	c := New("web", &fakeStore{svc: desired}, rt)

	cond := c.reconcileEgress(context.Background(), []string{newTarget}, desired)

	if rt.removeCalls != 1 {
		t.Errorf("removeCalls = %d, want 1 (the orphaned old-image sidecar)", rt.removeCalls)
	}
	if _, ok := rt.containers[egressSidecarName(oldTarget)]; ok {
		t.Error("old sidecar still present, want removed")
	}
	// The new target's app container was never seeded running, so the
	// new sidecar can't attach yet: cleanup and creation are independent
	// concerns, this proves cleanup happens regardless of the other.
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "PolicyApplyFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=PolicyApplyFailed (new target not running yet)", cond)
	}
}

func TestReconcileEgress_OptOutRemovesSidecar(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80} // no Egress: opted out
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)
	appState, _ := rt.InspectByName(context.Background(), target)
	rt.seedLabeled(egressSidecarName(target), true, map[string]string{egressTargetIDLabelKey: appState.ID})
	c := New("web", &fakeStore{svc: desired}, rt)

	cond := c.reconcileEgress(context.Background(), []string{target}, desired)

	if cond.Status != reconcile.ConditionUnknown || cond.Reason != "NotConfigured" {
		t.Errorf("condition = %+v, want Status=Unknown Reason=NotConfigured", cond)
	}
	if _, ok := rt.containers[egressSidecarName(target)]; ok {
		t.Error("sidecar still present after opting out, want removed")
	}
	// The app container itself is untouched: opting out of egress must
	// never affect the app's own container.
	if _, ok := rt.containers[target]; !ok {
		t.Error("app container removed, want it untouched by an egress opt-out")
	}
}

func TestReconcileEgress_NoTargets_RemovesSidecarsAndReportsUnknown(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Egress: allowlistPolicy(store.ServiceEgressAllow{Host: "api.example.com", Port: 443})}
	target := ContainerName("web", desired.Image, "")
	rt.seedLabeled(egressSidecarName(target), true, map[string]string{egressTargetIDLabelKey: "some-id"})
	c := New("web", &fakeStore{svc: desired}, rt)

	// Suspended (removeStale already cleared every app container) calls
	// this with targets=nil.
	cond := c.reconcileEgress(context.Background(), nil, desired)

	if cond.Status != reconcile.ConditionUnknown || cond.Reason != "NoRunningTargets" {
		t.Errorf("condition = %+v, want Status=Unknown Reason=NoRunningTargets", cond)
	}
	if _, ok := rt.containers[egressSidecarName(target)]; ok {
		t.Error("sidecar still present with no running targets, want removed")
	}
}

func TestReconcileEgress_MultiReplica_OneSidecarPerTarget(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Replicas: 2, Egress: allowlistPolicy(store.ServiceEgressAllow{Host: "api.example.com", Port: 443})}
	target0 := ContainerName("web", desired.Image, "")
	target1 := replicaContainerName("web", desired.Image, "", 1)
	rt.seed(target0, true)
	rt.seed(target1, true)
	c := New("web", &fakeStore{svc: desired}, rt)

	cond := c.reconcileEgress(context.Background(), []string{target0, target1}, desired)

	if cond.Status != reconcile.ConditionTrue || cond.Reason != "PoliciesApplied" {
		t.Fatalf("condition = %+v, want Status=True Reason=PoliciesApplied", cond)
	}
	if rt.createCalls != 2 {
		t.Fatalf("createCalls = %d, want 2 (one sidecar per replica)", rt.createCalls)
	}
	for _, target := range []string{target0, target1} {
		if _, ok := rt.containers[egressSidecarName(target)]; !ok {
			t.Errorf("missing sidecar for %q", target)
		}
	}
}

// TestController_Reconcile_Egress_ConditionAppended is the
// integration-level proof that a full Reconcile call actually wires
// reconcileEgress in, on top of egress_test.go's own direct unit tests
// of reconcileEgress: EgressPolicyReady always shows up as the second
// condition, after the ordinary Ready one.
func TestController_Reconcile_Egress_ConditionAppended(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Egress: allowlistPolicy(store.ServiceEgressAllow{Host: "api.example.com", Port: 443}),
	}
	c := New("web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(result.Conditions) != 2 {
		t.Fatalf("Conditions = %+v, want 2 (Ready, EgressPolicyReady)", result.Conditions)
	}
	if result.Conditions[0].Type != "Ready" || result.Conditions[0].Status != reconcile.ConditionTrue {
		t.Errorf("Conditions[0] = %+v, want a successful Ready condition", result.Conditions[0])
	}
	egressCond := result.Conditions[1]
	if egressCond.Type != "EgressPolicyReady" || egressCond.Status != reconcile.ConditionTrue || egressCond.Reason != "PoliciesApplied" {
		t.Errorf("Conditions[1] = %+v, want Type=EgressPolicyReady Status=True Reason=PoliciesApplied", egressCond)
	}
	if rt.createCalls != 2 {
		t.Errorf("createCalls = %d, want 2 (app container plus its egress sidecar)", rt.createCalls)
	}
}

func TestController_Reconcile_Suspended_RemovesEgressSidecar(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Suspended: true,
		Egress: allowlistPolicy(store.ServiceEgressAllow{Host: "api.example.com", Port: 443}),
	}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)
	appState, _ := rt.InspectByName(context.Background(), target)
	rt.seedLabeled(egressSidecarName(target), true, map[string]string{egressTargetIDLabelKey: appState.ID})
	c := New("web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(result.Conditions) != 2 || result.Conditions[1].Reason != "NoRunningTargets" {
		t.Errorf("Conditions = %+v, want [Suspended, EgressPolicyReady(NoRunningTargets)]", result.Conditions)
	}
	if _, ok := rt.containers[egressSidecarName(target)]; ok {
		t.Error("sidecar still present after suspend, want removed")
	}
}

func TestController_Teardown_RemovesEgressSidecar(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Egress: allowlistPolicy(store.ServiceEgressAllow{Host: "api.example.com", Port: 443})}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)
	appState, _ := rt.InspectByName(context.Background(), target)
	rt.seedLabeled(egressSidecarName(target), true, map[string]string{egressTargetIDLabelKey: appState.ID})
	c := New("web", &fakeStore{svc: desired}, rt)

	if err := c.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown() error = %v", err)
	}
	if _, ok := rt.containers[target]; ok {
		t.Error("app container still present after Teardown")
	}
	if _, ok := rt.containers[egressSidecarName(target)]; ok {
		t.Error("egress sidecar still present after Teardown")
	}
}

// TestReconcileEgress_ScopedToOwningService proves egressSidecars never
// picks up a differently-named service's own sidecar purely because of
// a shared prefix, the same boundary ownsContainer already establishes
// for ordinary app containers (staleContainers' own doc comment).
func TestReconcileEgress_ScopedToOwningService(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80} // no Egress
	webTarget := ContainerName("web", desired.Image, "")
	rt.seed(webTarget, true)

	// "web-worker" would prefix-match "web-" but is a different service.
	otherDesired := &store.DesiredService{Name: "web-worker", Image: "img:v1", Port: 80}
	otherTarget := ContainerName("web-worker", otherDesired.Image, "")
	rt.seed(otherTarget, true)
	rt.seedLabeled(egressSidecarName(otherTarget), true, map[string]string{egressTargetIDLabelKey: "irrelevant"})

	c := New("web", &fakeStore{svc: desired}, rt)
	sidecars, err := c.egressSidecars(context.Background())
	if err != nil {
		t.Fatalf("egressSidecars() error = %v", err)
	}
	for _, cs := range sidecars {
		if strings.HasPrefix(cs.Name, "web-worker") {
			t.Errorf("egressSidecars() for service %q returned a %q sidecar, want scoped to its own service", "web", cs.Name)
		}
	}
}

// TestReconcileEgress_FreshSidecar_NotReportedAppliedUntilMarkerSeen is
// the direct proof for the false-"enforced" bug found in real testing: a
// freshly created sidecar whose boot script hasn't yet written
// egressReadyMarkerPath (execFailCount simulates it still mid-setup,
// analogous to the real "stuck mid-apk-add" case with the pre-fix
// image) must not be reported PoliciesApplied just because Create+Start
// succeeded. Once the marker does appear (fail count exhausted),
// waitEgressReady's own poll picks that up within its budget and the
// condition does flip to applied, proving this isn't just "always
// false" either.
func TestReconcileEgress_FreshSidecar_NotReportedAppliedUntilMarkerSeen(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.execFailCount = 2 // first two readiness checks see no marker yet
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Egress: allowlistPolicy(store.ServiceEgressAllow{Host: "api.example.com", Port: 443}),
	}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)
	c := New("web", &fakeStore{svc: desired}, rt,
		WithEgressReadyBudget(200*time.Millisecond),
		WithEgressReadyPollInterval(2*time.Millisecond),
	)

	cond := c.reconcileEgress(context.Background(), []string{target}, desired)

	if cond.Status != reconcile.ConditionTrue || cond.Reason != "PoliciesApplied" {
		t.Fatalf("condition = %+v, want Status=True Reason=PoliciesApplied once the marker check finally succeeds", cond)
	}
	if len(rt.execCalls) < 3 {
		t.Fatalf("execCalls = %d, want >= 3 (2 not-ready checks plus the one that finally succeeded): PoliciesApplied must be earned by a real check, not assumed from Create+Start alone", len(rt.execCalls))
	}
}

// TestReconcileEgress_StuckBootScript_ReportsPolicyApplyFailed_NotSilently
// proves a sidecar whose boot script never finishes (egressRulesInstalled
// permanently reports the marker missing, the real "hung apk add, no
// iptables rule ever installed" scenario) surfaces as PolicyApplyFailed
// after a bounded wait rather than a false PoliciesApplied, an
// indefinite hang, or silence.
func TestReconcileEgress_StuckBootScript_ReportsPolicyApplyFailed_NotSilently(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.execExitCode = 1 // marker check never succeeds: the sidecar never finishes booting
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Egress: allowlistPolicy(store.ServiceEgressAllow{Host: "api.example.com", Port: 443}),
	}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)
	c := New("web", &fakeStore{svc: desired}, rt,
		WithEgressReadyBudget(20*time.Millisecond),
		WithEgressReadyPollInterval(2*time.Millisecond),
	)

	done := make(chan reconcile.Condition, 1)
	go func() { done <- c.reconcileEgress(context.Background(), []string{target}, desired) }()

	select {
	case cond := <-done:
		if cond.Status != reconcile.ConditionFalse || cond.Reason != "PolicyApplyFailed" {
			t.Fatalf("condition = %+v, want Status=False Reason=PolicyApplyFailed for a sidecar stuck mid-boot", cond)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reconcileEgress did not return within 2s: a stuck sidecar must fail within its bounded readiness budget, not hang")
	}
	if rt.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1: a stuck-but-running sidecar must not be repeatedly recreated", rt.createCalls)
	}
}

// TestReconcileEgress_AlreadyExists_NotYetReady_NotReportedApplied is
// the steady-state counterpart of the marker-check proof above: a
// sidecar that already exists, is Running, and carries the correct
// egressTargetIDLabelKey (exactly TestReconcileEgress_AlreadyApplied_NoOp's
// own setup, which the pre-fix code reported PoliciesApplied for purely
// on that basis) must still be checked for its readiness marker on every
// pass, not trusted just because it looks structurally correct.
func TestReconcileEgress_AlreadyExists_NotYetReady_NotReportedApplied(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Egress: allowlistPolicy(store.ServiceEgressAllow{Host: "api.example.com", Port: 443})}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)
	appState, _ := rt.InspectByName(context.Background(), target)
	rt.seedLabeled(egressSidecarName(target), true, map[string]string{egressTargetIDLabelKey: appState.ID})
	rt.execExitCode = 1 // marker missing: the sidecar is running but hasn't finished installing rules
	c := New("web", &fakeStore{svc: desired}, rt)

	cond := c.reconcileEgress(context.Background(), []string{target}, desired)

	if cond.Status != reconcile.ConditionFalse || cond.Reason != "PolicyApplyFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=PolicyApplyFailed: a running, correctly-labeled sidecar without a confirmed marker must not be reported applied", cond)
	}
	if rt.createCalls != 0 || rt.removeCalls != 0 {
		t.Errorf("createCalls=%d removeCalls=%d, want 0/0: a not-yet-ready sidecar is still converging, not stale, so it must not be recreated", rt.createCalls, rt.removeCalls)
	}
}
