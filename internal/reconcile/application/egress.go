package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// egressImage is a stock, widely-used "network toolbox" image
// (nicolaka/netshoot, 100M+ pulls) rather than a control-plane-built
// custom one: this still needs no BuildKit-at-startup capability, the
// same "pull a public image, run a script via Command" shape
// internal/reconcile/registry's htpasswdBootScript and
// internal/reconcile/cloudflaretunnel already establish for their own
// single-purpose containers. Picked over plain alpine specifically
// because it ships iptables, ipset, and bind-tools (dig/nslookup)
// preinstalled: egressBootScript below installs enforcement rules
// immediately at boot with no package-manager network call, so a slow
// or unreachable apk mirror can never hang egress setup (or, observed
// once in real testing, the app container's own networking) the way an
// `apk add` on every sidecar (re)creation could.
const egressImage = "nicolaka/netshoot:v0.16"

// egressSidecarSuffix names the egress sidecar container derived from
// its app container's own name: "<target>-egress". Deliberately a
// suffix, not a name ownsContainer's own hash-shape check would ever
// match (that check only ever matches serviceName + hash [+ "-rN"]), so
// listing this service's own app containers (staleContainers,
// ListByPrefix(serviceName+"-")) never accidentally includes or excludes
// a sidecar it wasn't looking for.
const egressSidecarSuffix = "-egress"

// egressTargetIDLabelKey records, on the sidecar container itself, which
// app container ID it shares a network namespace with
// (NetworkMode: "container:<id>"). A name match alone isn't enough to
// prove a running sidecar is still correctly attached: an operator
// manually removing and Levelrail self-healing an app container keeps
// the same deterministic name (ContainerName hashes the image, not a
// container ID) but gets a brand new ID, which would silently strand an
// existing sidecar pointed at a netns that no longer has a live owner.
// Comparing this label against the app container's current ID on every
// pass is what makes that case detected and recreated instead of
// silently stale.
const egressTargetIDLabelKey = appspec.ReservedLabelPrefix + "egress-target-id"

// egressAllowEnv/egressResolveIntervalEnv name the env vars
// egressBootScript reads at container start and on every resolve loop
// iteration.
const (
	egressAllowEnv           = "LEVELRAIL_EGRESS_ALLOW"
	egressResolveIntervalEnv = "LEVELRAIL_EGRESS_RESOLVE_INTERVAL_SECONDS"
)

// egressDefaultResolveIntervalSeconds is how often the sidecar
// re-resolves every allowed host and refreshes the ipset: DNS records
// aren't static, so a one-shot resolve at container start would let an
// allowed host's rotated IP silently fall outside the allowlist.
const egressDefaultResolveIntervalSeconds = "30"

const egressIPSetName = "levelrail_egress"

// defaultEgressReadyBudget bounds how long waitEgressReady waits, after
// starting a freshly (re)created sidecar, for egressReadyMarkerPath to
// appear before giving up: a stuck sidecar must surface as
// PolicyApplyFailed within a bounded time, never report PoliciesApplied
// on trust alone, and never hang Reconcile indefinitely. Generous
// relative to how fast egressBootScript actually runs (no package
// install, egressImage's own doc comment) because it only pays this
// cost once, when a sidecar is first created.
const defaultEgressReadyBudget = 30 * time.Second

// defaultEgressReadyPollInterval is how often waitEgressReady rechecks
// a not-yet-ready sidecar during its defaultEgressReadyBudget wait.
const defaultEgressReadyPollInterval = 500 * time.Millisecond

// egressCheckTimeout bounds a single egress-readiness Exec call
// (checkEgressRulesInstalled), so one wedged call can never hang a
// caller past this: the real incident that motivated this whole
// verification step was a syscall stuck inside a sidecar that never
// returned at all.
const egressCheckTimeout = 5 * time.Second

// egressReadyMarkerPath is the file egressBootScript creates the moment
// the OUTPUT DROP policy and ipset are actually installed. The
// reconciler execs a check for this file (egressRulesInstalled) rather
// than trusting Create+Start succeeding: a running container only
// proves the process started, not that its setup finished, and this is
// the boot script's own positive signal that it has.
const egressReadyMarkerPath = "/run/levelrail-egress-ready"

// egressBootScript installs the enforcement rules once, resolves the
// allowlist into the ipset synchronously so egressReadyMarkerPath is
// only ever written once both halves of enforcement (the DROP policy
// and a populated ipset, not just the former) are actually in place,
// then loops forever re-resolving on an interval. The iptables rule set
// itself is installed exactly once and never touched again: the ipset
// it matches against is swapped, so a resolve cycle never leaves a
// window where the OUTPUT chain has no rules installed at all.
//
// No package install here: egressImage already ships iptables, ipset,
// and getent, so this runs immediately with no network dependency at
// boot (egressImage's own doc comment).
//
// Baseline traffic kept open, conservatively, so this can never make an
// app unreachable purely by enforcing egress: loopback (lo), replies to
// any connection the app itself received (ESTABLISHED,RELATED, which
// covers inbound traffic from Caddy ingress and its own responses, since
// egress enforcement only ever touches the OUTPUT chain, never INPUT),
// and DNS (port 53, both protocols, to any resolver) since without it no
// host in the allowlist could ever be resolved in the first place.
//
// A real limitation, not solved here: there is a window between the app
// container starting (open egress, Docker's own default) and this
// script finishing its first ipset/iptables setup where egress is
// briefly unrestricted. Closing that race would need the rules installed
// before the app's own process starts taking traffic, which the sidecar
// model (a second container joining the netns after the first exists)
// cannot guarantee order for. Documented, not silently accepted.
//
// Another: enforcement is IPv4 only (iptables, not ip6tables). An
// allowed host's IPv6 (AAAA) addresses are resolved and deliberately
// skipped, not added to this IPv4-only ipset; if the app container ever
// has IPv6 connectivity, that path is unenforced.
const egressBootScript = `set -e

IPSET=` + egressIPSetName + `
ipset create "$IPSET" hash:ip,port -exist
ipset create "${IPSET}_tmp" hash:ip,port -exist

iptables -F OUTPUT
iptables -P OUTPUT DROP
iptables -A OUTPUT -o lo -j ACCEPT
iptables -A OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
iptables -A OUTPUT -p udp --dport 53 -j ACCEPT
iptables -A OUTPUT -p tcp --dport 53 -j ACCEPT
iptables -A OUTPUT -m set --match-set "$IPSET" dst,dst -j ACCEPT

resolve_allowlist() {
  ipset flush "${IPSET}_tmp"
  for entry in $` + egressAllowEnv + `; do
    host="${entry%%:*}"
    port="${entry##*:}"
    for ip in $(getent ahostsv4 "$host" 2>/dev/null | awk '{print $1}'); do
      case "$ip" in
        *:*) continue ;; # this ipset is IPv4-only (hash:ip,port defaults to family inet) and no ip6tables rules are installed, so an IPv6 address here would abort the whole script under set -e; ahostsv4 already filters to A records, this is defense in depth
      esac
      ipset add "${IPSET}_tmp" "$ip,$port" -exist
    done
  done
  ipset swap "$IPSET" "${IPSET}_tmp"
}

resolve_allowlist
touch ` + egressReadyMarkerPath + `

INTERVAL="${` + egressResolveIntervalEnv + `:-` + egressDefaultResolveIntervalSeconds + `}"

while true; do
  sleep "$INTERVAL"
  resolve_allowlist
done
`

// egressSidecarName derives the sidecar container's name from the app
// container name it shares a network namespace with.
func egressSidecarName(target string) string {
	return target + egressSidecarSuffix
}

// isEgressSidecarName reports whether name could be
// egressSidecarName(target) for some target this service owns, and
// returns that target. The inverse of egressSidecarName, used to
// recognize this service's own sidecars inside a ListByPrefix result
// without a second, parallel naming scheme to keep in sync.
func isEgressSidecarName(serviceName, name string) (target string, ok bool) {
	target, ok = strings.CutSuffix(name, egressSidecarSuffix)
	if !ok || !ownsContainer(serviceName, target) {
		return "", false
	}
	return target, true
}

// egressAllowEnvValue renders allow as egressBootScript's own
// space-separated "host:port host:port" input format.
func egressAllowEnvValue(allow []store.ServiceEgressAllow) string {
	parts := make([]string, len(allow))
	for i, a := range allow {
		parts[i] = a.Host + ":" + strconv.Itoa(a.Port)
	}
	return strings.Join(parts, " ")
}

// egressSidecarSpec builds the sidecar's desired container spec: NET_ADMIN
// to install iptables/ipset rules, and NetworkMode pointing at
// appContainerID so those rules apply to the app container's own traffic,
// not the sidecar's (a sidecar on its own netns would only restrict
// itself).
func egressSidecarSpec(target, appContainerID string, policy *store.ServiceEgressPolicy) docker.ContainerSpec {
	return docker.ContainerSpec{
		Name:        egressSidecarName(target),
		Image:       egressImage,
		Entrypoint:  []string{"sh", "-c"},
		Command:     []string{egressBootScript},
		CapAdd:      []string{"NET_ADMIN"},
		NetworkMode: "container:" + appContainerID,
		Env: map[string]string{
			egressAllowEnv:           egressAllowEnvValue(policy.Allow),
			egressResolveIntervalEnv: egressDefaultResolveIntervalSeconds,
		},
		Labels: map[string]string{egressTargetIDLabelKey: appContainerID},
	}
}

// egressSidecars lists every egress sidecar this service (and, when
// instance-ownership checking is configured, this control-plane
// instance) currently owns, regardless of which app-container generation
// they were created for: reconcileEgress diffs this against the current
// desired target set itself.
func (c *Controller) egressSidecars(ctx context.Context) ([]docker.ContainerState, error) {
	all, err := c.runtime.ListByPrefix(ctx, c.serviceName+"-")
	if err != nil {
		return nil, fmt.Errorf("list containers for %s: %w", c.serviceName, err)
	}
	return c.filterEgressSidecars(all), nil
}

// filterEgressSidecars is egressSidecars' pure filtering half.
func (c *Controller) filterEgressSidecars(all []docker.ContainerState) []docker.ContainerState {
	var out []docker.ContainerState
	for _, cs := range all {
		if _, ok := isEgressSidecarName(c.serviceName, cs.Name); !ok || !c.ownsInstance(cs) {
			continue
		}
		out = append(out, cs)
	}
	return out
}

// appendEgressCondition runs egress reconciliation and appends its
// result to result.Conditions, called from Reconcile after each
// strategy's own convergence step: egress is applied to whatever's
// actually running by the time this runs, independent of whether that
// step itself reported success, the same level-triggered "always attempt
// convergence, report what's true" discipline every other step here
// already follows.
func (c *Controller) appendEgressCondition(ctx context.Context, result reconcile.Result, targets []string, desired *store.DesiredService) reconcile.Result {
	result.Conditions = append(result.Conditions, c.reconcileEgress(ctx, targets, desired))
	return result
}

// reconcileEgress converges this service's egress sidecars to match
// desired.Egress: one sidecar per currently-desired target container,
// each network-namespace-shared with the app container it protects.
// Idempotent and level-triggered like every other step in this
// controller: every pass re-derives what should exist from desired.Egress
// and targets, never from what a previous pass did.
//
// Deliberately its own step, run after the strategy switch has already
// converged targets to running containers, rather than folded into
// createAndStart: a sidecar can only join a netns that already exists,
// so it always follows its app container, never leads it (see
// egressBootScript's own doc comment for the resulting, documented,
// unrestricted-egress startup window this ordering can't avoid).
func (c *Controller) reconcileEgress(ctx context.Context, targets []string, desired *store.DesiredService) reconcile.Condition {
	existing, err := c.egressSidecars(ctx)
	if err != nil {
		return egressNotReady("PolicyApplyFailed", err)
	}
	return c.reconcileEgressWith(ctx, existing, targets, desired)
}

// reconcileEgressWith is reconcileEgress given an already-listed sidecar set.
func (c *Controller) reconcileEgressWith(ctx context.Context, existing []docker.ContainerState, targets []string, desired *store.DesiredService) reconcile.Condition {
	allowlisted := desired.Egress != nil && desired.Egress.Mode == store.EgressModeAllowlist

	wanted := make(map[string]bool, len(targets))
	if allowlisted {
		for _, target := range targets {
			wanted[egressSidecarName(target)] = true
		}
	}

	var firstErr error
	for _, cs := range existing {
		if wanted[cs.Name] {
			continue
		}
		if err := c.removeContainers(ctx, []docker.ContainerState{cs}); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("remove orphaned egress sidecar %q: %w", cs.Name, err)
		}
	}

	switch {
	case firstErr != nil:
		return egressNotReady("PolicyApplyFailed", firstErr)
	case !allowlisted:
		return reconcile.Condition{Type: "EgressPolicyReady", Status: reconcile.ConditionUnknown, Reason: "NotConfigured"}
	case len(targets) == 0:
		// Suspended, or every replica scaled to zero: nothing is running
		// to protect, so "PoliciesApplied" would be a false claim even
		// though the loop below would vacuously succeed over an empty
		// target list.
		return reconcile.Condition{Type: "EgressPolicyReady", Status: reconcile.ConditionUnknown, Reason: "NoRunningTargets"}
	}

	existingByName := make(map[string]docker.ContainerState, len(existing))
	for _, cs := range existing {
		existingByName[cs.Name] = cs
	}

	allApplied := true
	var pending []string
	for _, target := range targets {
		applied, err := c.ensureEgressSidecar(ctx, target, existingByName[egressSidecarName(target)], desired.Egress)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if !applied {
			allApplied = false
			pending = append(pending, target)
		}
	}

	if firstErr != nil {
		return egressNotReady("PolicyApplyFailed", firstErr)
	}
	if !allApplied {
		return reconcile.Condition{
			Type: "EgressPolicyReady", Status: reconcile.ConditionFalse, Reason: "PolicyApplyFailed",
			Message: "waiting for the app container to be running and its egress sidecar to confirm enforcement rules are installed for: " + strings.Join(pending, ", "),
		}
	}
	return reconcile.Condition{Type: "EgressPolicyReady", Status: reconcile.ConditionTrue, Reason: "PoliciesApplied"}
}

// effectiveEgressReadyBudget returns c.egressReadyBudget
// (WithEgressReadyBudget's construction-time value) or
// defaultEgressReadyBudget when that option was never applied.
func (c *Controller) effectiveEgressReadyBudget() time.Duration {
	if c.egressReadyBudget > 0 {
		return c.egressReadyBudget
	}
	return defaultEgressReadyBudget
}

// effectiveEgressReadyPollInterval returns c.egressReadyPollInterval
// (WithEgressReadyPollInterval's construction-time value) or
// defaultEgressReadyPollInterval when that option was never applied.
func (c *Controller) effectiveEgressReadyPollInterval() time.Duration {
	if c.egressReadyPollInterval > 0 {
		return c.egressReadyPollInterval
	}
	return defaultEgressReadyPollInterval
}

// egressRulesInstalled execs a check for egressReadyMarkerPath inside
// the sidecar, the boot script's own positive signal that the OUTPUT
// DROP policy and ipset are actually in place, not just that the
// container process is running. A missing marker (the exec runs and
// exits nonzero) is a normal "still converging" observation, not a
// failure: only a genuine Exec failure (the sidecar died, the runtime is
// unreachable) is returned as an error.
func (c *Controller) egressRulesInstalled(ctx context.Context, sidecarID string) (ready bool, err error) {
	rc, err := c.runtime.Exec(ctx, sidecarID, []string{"test", "-f", egressReadyMarkerPath})
	if err != nil {
		return false, err
	}
	defer func() { _ = rc.Close() }()

	_, readErr := io.Copy(io.Discard, rc)
	var execErr *docker.ExecExitError
	switch {
	case readErr == nil:
		return true, nil
	case errors.As(readErr, &execErr):
		return false, nil
	default:
		return false, readErr
	}
}

// checkEgressRulesInstalled is egressRulesInstalled bounded by
// egressCheckTimeout, so a single wedged Exec call (the exact failure
// mode a stuck sidecar produced in real testing: a syscall that never
// returns) can never hang a caller past that bound.
func (c *Controller) checkEgressRulesInstalled(ctx context.Context, sidecarID string) (bool, error) {
	checkCtx, cancel := context.WithTimeout(ctx, egressCheckTimeout)
	defer cancel()
	return c.egressRulesInstalled(checkCtx, sidecarID)
}

// waitEgressReady blocks, up to egressReadyBudget, until sidecarID's
// boot script reports its enforcement rules installed
// (checkEgressRulesInstalled), polling every egressReadyPollInterval.
// Mirrors waitReady's own bounded-wait-after-(re)start shape
// (controller.go): a freshly (re)created sidecar gets a real budget to
// finish booting before it can be reported PoliciesApplied, and a
// sidecar that never finishes within that budget returns false, not an
// error, so it surfaces through reconcileEgress as PolicyApplyFailed
// rather than hanging Reconcile itself.
func (c *Controller) waitEgressReady(ctx context.Context, sidecarID string) (bool, error) {
	deadline := time.Now().Add(c.effectiveEgressReadyBudget())
	for {
		ready, err := c.checkEgressRulesInstalled(ctx, sidecarID)
		if err != nil {
			return false, err
		}
		if ready || time.Now().After(deadline) {
			return ready, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(c.effectiveEgressReadyPollInterval()):
		}
	}
}

// ensureEgressSidecar converges one target's sidecar. existing is the
// zero value when no sidecar by that name exists yet. Returns applied
// true once a sidecar correctly attached to target's current container
// ID is running AND has confirmed (egressRulesInstalled) its
// enforcement rules are actually installed; false means this pass could
// not get there, either because the app container isn't up yet or the
// sidecar hasn't finished booting, not a hard failure in either case.
func (c *Controller) ensureEgressSidecar(ctx context.Context, target string, existing docker.ContainerState, policy *store.ServiceEgressPolicy) (applied bool, err error) {
	appState, err := c.runtime.InspectByName(ctx, target)
	if err != nil {
		return false, fmt.Errorf("inspect %q for egress sidecar: %w", target, err)
	}
	if appState == nil || !appState.Running {
		// ensureReplicaRunning owns getting the app container running;
		// this just isn't the pass that attaches a sidecar to a netns
		// that doesn't exist yet.
		return false, nil
	}

	sidecarName := egressSidecarName(target)
	if existing.Name == sidecarName {
		if existing.Running && existing.Labels[egressTargetIDLabelKey] == appState.ID {
			ready, err := c.checkEgressRulesInstalled(ctx, existing.ID)
			if err != nil {
				return false, fmt.Errorf("check egress sidecar %q ready: %w", sidecarName, err)
			}
			return ready, nil
		}
		// Either stopped, or still pointed at a since-replaced app
		// container ID (self-healing after a manual removal): stale
		// either way, remove before recreating.
		if err := c.removeContainers(ctx, []docker.ContainerState{existing}); err != nil {
			return false, fmt.Errorf("remove stale egress sidecar %q: %w", sidecarName, err)
		}
	}

	spec := egressSidecarSpec(target, appState.ID, policy)
	id, err := c.runtime.Create(ctx, spec)
	if err != nil {
		return false, fmt.Errorf("create egress sidecar %q: %w", sidecarName, err)
	}
	if err := c.runtime.Start(ctx, id); err != nil {
		return false, fmt.Errorf("start egress sidecar %q: %w", sidecarName, err)
	}
	ready, err := c.waitEgressReady(ctx, id)
	if err != nil {
		return false, fmt.Errorf("wait for egress sidecar %q to install rules: %w", sidecarName, err)
	}
	return ready, nil
}

// removeAllEgressSidecars removes every egress sidecar this service owns,
// regardless of target: Teardown's own counterpart to removeStale(nil)
// for app containers, since a deleted service's egress sidecars aren't
// matched by that call's own ownsContainer-based filtering (sidecar
// names deliberately fall outside that shape, egressSidecarSuffix's own
// doc comment).
func (c *Controller) removeAllEgressSidecars(ctx context.Context) error {
	existing, err := c.egressSidecars(ctx)
	if err != nil {
		return err
	}
	return c.removeContainers(ctx, existing)
}

func egressNotReady(reason string, err error) reconcile.Condition {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return reconcile.Condition{Type: "EgressPolicyReady", Status: reconcile.ConditionFalse, Reason: reason, Message: msg}
}
