package privnet

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Source is the store surface the controller and the API both read.
type Source interface {
	ListApps(ctx context.Context) ([]store.App, error)
	ListPrivateNetworks(ctx context.Context) ([]store.PrivateNetwork, error)
	ListPrivateNetworkRules(ctx context.Context) ([]store.PrivateNetworkRule, error)
}

// Runtime is the Docker surface the controller needs.
type Runtime interface {
	EnsureNetwork(ctx context.Context, name string) (string, error)
	RemoveNetwork(ctx context.Context, name string) error
	ListNetworksByPrefix(ctx context.Context, prefix string) ([]docker.NetworkInfo, error)
	ListByPrefix(ctx context.Context, prefix string) ([]docker.ContainerState, error)
	NetworkConnect(ctx context.Context, network, containerID string) error
	NetworkDisconnect(ctx context.Context, network, containerID string, force bool) error
}

// Controller keeps Docker networks in line with private networks and rules.
type Controller struct {
	src        Source
	rt         Runtime
	prefix     string
	instanceID string
	nodeID     string
}

// NewController builds the enforcer. prefix must match the application
// controller's network prefix so per-app networks resolve to app IDs.
func NewController(src Source, rt Runtime, prefix, instanceID string) *Controller {
	return &Controller{src: src, rt: rt, prefix: application.NetworkPrefixOrDefault(prefix), instanceID: instanceID}
}

// ForNode names the controller for one remote node's runtime.
func (c *Controller) ForNode(nodeID string) *Controller {
	cp := *c
	cp.nodeID = nodeID
	return &cp
}

// Name implements reconcile.Controller.
func (c *Controller) Name() string {
	if c.nodeID != "" {
		return "privnet/" + c.nodeID
	}
	return "privnet"
}

// Reconcile implements reconcile.Controller.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	nets, rules, apps, err := Load(ctx, c.src)
	if err != nil {
		return result(false, "LoadFailed", err), err
	}
	containers, err := c.rt.ListByPrefix(ctx, "")
	if err != nil {
		return result(false, "ListContainersFailed", err), fmt.Errorf("privnet: list containers: %w", err)
	}
	owner := AppOwners(c.prefix, apps, containers)
	links := Links(c.prefix, nets, rules)
	want := make(map[string]map[string]bool, len(links))
	for _, l := range links {
		set := make(map[string]bool, len(l.Apps))
		for _, a := range l.Apps {
			set[a] = true
		}
		want[l.Name] = set
	}

	var firstErr error
	note := func(err error) {
		if err != nil {
			slog.Warn("privnet reconcile step failed", slog.String("controller", c.Name()), slog.String("error", err.Error()))
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	for _, l := range links {
		if _, err := c.rt.EnsureNetwork(ctx, l.Name); err != nil {
			note(fmt.Errorf("ensure network %q: %w", l.Name, err))
			continue
		}
		for _, ct := range containers {
			if !ct.Running || !want[l.Name][owner[ct.ID]] {
				continue
			}
			if !attached(ct, l.Name) {
				note(c.rt.NetworkConnect(ctx, l.Name, ct.ID))
			}
		}
	}

	observed, err := c.rt.ListNetworksByPrefix(ctx, c.prefix+"-p")
	if err != nil {
		note(fmt.Errorf("list private networks: %w", err))
	}
	for _, n := range observed {
		if !c.managed(n) {
			continue
		}
		members, keep := want[n.Name]
		for _, ct := range containers {
			if attached(ct, n.Name) && (!keep || !members[owner[ct.ID]]) {
				note(c.rt.NetworkDisconnect(ctx, n.Name, ct.ID, true))
			}
		}
		if !keep {
			note(c.rt.RemoveNetwork(ctx, n.Name))
		}
	}

	if firstErr != nil {
		return result(false, "EnforceFailed", firstErr), firstErr
	}
	return result(true, "Enforced", nil), nil
}

func (c *Controller) managed(n docker.NetworkInfo) bool {
	if !strings.HasPrefix(n.Name, c.prefix+"-pn-") && !strings.HasPrefix(n.Name, c.prefix+"-pl-") {
		return false
	}
	owner, ok := n.Labels[spec.InstanceLabelKey]
	return c.instanceID == "" || !ok || owner == c.instanceID
}

func attached(ct docker.ContainerState, network string) bool {
	for _, n := range ct.Networks {
		if n.Name == network {
			return true
		}
	}
	return false
}

func result(ok bool, reason string, err error) reconcile.Result {
	status := reconcile.ConditionTrue
	if !ok {
		status = reconcile.ConditionFalse
	}
	cond := reconcile.Condition{Type: reconcile.ConditionTypeReady, Status: status, Reason: reason}
	if err != nil {
		cond.Message = err.Error()
	}
	return reconcile.Result{Conditions: []reconcile.Condition{cond}}
}

// Load reads networks, rules and apps from the store in evaluator form.
func Load(ctx context.Context, src Source) ([]Network, []Rule, []store.App, error) {
	pn, err := src.ListPrivateNetworks(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("privnet: %w", err)
	}
	pr, err := src.ListPrivateNetworkRules(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("privnet: %w", err)
	}
	apps, err := src.ListApps(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("privnet: %w", err)
	}
	nets := make([]Network, 0, len(pn))
	for _, n := range pn {
		nets = append(nets, Network{ID: n.ID, Name: n.Name, DefaultDeny: n.DefaultDeny, Members: n.Members})
	}
	rules := make([]Rule, 0, len(pr))
	for _, r := range pr {
		rules = append(rules, Rule{ID: r.ID, From: r.FromApp, To: r.ToApp})
	}
	return nets, rules, apps, nil
}

// AppOwners maps container ID to app ID using the per-app network each
// container is attached to.
func AppOwners(prefix string, apps []store.App, containers []docker.ContainerState) map[string]string {
	byNet := make(map[string]string, len(apps))
	for _, a := range apps {
		byNet[application.NetworkName(prefix, a.ID)] = a.ID
	}
	out := map[string]string{}
	for _, ct := range containers {
		for _, n := range ct.Networks {
			if id, ok := byNet[n.Name]; ok {
				out[ct.ID] = id
				break
			}
		}
	}
	return out
}

// SharedNetworks returns, per unordered app pair, whether some container of
// each app is attached to one common Docker network besides per-app ones.
func SharedNetworks(prefix string, apps []store.App, containers []docker.ContainerState) map[[2]string]bool {
	owner := AppOwners(prefix, apps, containers)
	nets := map[string]map[string]bool{}
	for _, ct := range containers {
		app, ok := owner[ct.ID]
		if !ok {
			continue
		}
		for _, n := range ct.Networks {
			if !strings.HasPrefix(n.Name, prefix+"-p") {
				continue
			}
			if nets[n.Name] == nil {
				nets[n.Name] = map[string]bool{}
			}
			nets[n.Name][app] = true
		}
	}
	out := map[[2]string]bool{}
	for _, members := range nets {
		for a := range members {
			for b := range members {
				if a < b {
					out[[2]string{a, b}] = true
				}
			}
		}
	}
	return out
}

// PairKey is the map key SharedNetworks uses for an unordered pair.
func PairKey(a, b string) [2]string { return pairKey(a, b) }
