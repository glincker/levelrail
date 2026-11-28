package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strings"

	"github.com/glincker/levelrail/internal/catalog"
	"github.com/glincker/levelrail/internal/failure"
	"github.com/glincker/levelrail/internal/gpu"
	"github.com/glincker/levelrail/internal/models"
	"github.com/glincker/levelrail/internal/store"
)

// platformPreflight compares a template's verified platforms and GPU need with
// the host that will run it. It returns a plain refusal, or "" when the install can go on.
// overridable is true only for an arch mismatch, which emulation may still serve.
func (rt *Router) platformPreflight(ctx context.Context, tpl catalog.Template) (reason string, overridable bool) {
	return rt.platformPreflightOn(ctx, tpl, nil)
}

// platformPreflightOn evaluates the same checks against a pinned target node's
// reported arch and GPU inventory; a nil target means the control plane host.
func (rt *Router) platformPreflightOn(ctx context.Context, tpl catalog.Template, target *nodeTarget) (reason string, overridable bool) {
	host, where := rt.containerPlatform(ctx), "this host"
	if target != nil && target.id != "" {
		where = fmt.Sprintf("node %q", target.name)
		host = rt.nodeContainerPlatform(ctx, target.id)
	}
	if host != "" && len(tpl.Platforms) > 0 && !containsString(tpl.Platforms, host) {
		return fmt.Sprintf("template %q has no %s build for %s (verified: %s). Pick a node with a matching architecture (--node), or pin another image tag in the template settings. To try it anyway under emulation, repeat with allow_platform_mismatch.", tpl.ID, host, where, strings.Join(tpl.Platforms, ", ")), true
	}
	if reason := rt.gpuTemplatePreflightOn(ctx, tpl, target); reason != "" {
		return reason, false
	}
	if reason := kernelModulePreflight(tpl); reason != "" {
		return reason, false
	}
	return "", false
}

// containerPlatform is the os/arch containers run on. Containers are always linux,
// so a darwin or windows control plane (Docker Desktop) maps to linux plus the daemon
// arch when the pinger reports one, else the process arch.
func (rt *Router) containerPlatform(ctx context.Context) string {
	arch := runtime.GOARCH
	if src, ok := rt.dockerPinger.(interface {
		DaemonArch(ctx context.Context) string
	}); ok {
		if a := normalizeDaemonArch(src.DaemonArch(ctx)); a != "" {
			arch = a
		}
	}
	return "linux/" + arch
}

// nodeContainerPlatform is the platform a remote node's agent reported, or ""
// when it has not reported one yet (the arch check is then skipped).
func (rt *Router) nodeContainerPlatform(ctx context.Context, nodeID string) string {
	if rt.nodes == nil {
		return ""
	}
	n, err := rt.nodes.GetNode(ctx, nodeID)
	if err != nil || n == nil {
		return ""
	}
	if a := normalizeDaemonArch(n.AgentArch); a != "" {
		return "linux/" + a
	}
	return ""
}

// normalizeDaemonArch maps docker info names (x86_64, aarch64) to GOARCH names.
func normalizeDaemonArch(a string) string {
	switch strings.ToLower(strings.TrimSpace(a)) {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	default:
		return strings.ToLower(strings.TrimSpace(a))
	}
}

// refuseIfTeardownPending turns the 409 a re-deploy hits right after an async
// app delete into a named, retryable answer.
func (rt *Router) refuseIfTeardownPending(w http.ResponseWriter, r *http.Request, name string) bool {
	src, ok := rt.apps.(interface {
		ListPendingTeardowns(ctx context.Context) ([]store.PendingTeardown, error)
	})
	if !ok {
		return false
	}
	rows, err := src.ListPendingTeardowns(r.Context())
	if err != nil {
		return false
	}
	for _, p := range rows {
		if p.Name == name {
			w.Header().Set("Retry-After", "5")
			writeError(w, http.StatusConflict, "the earlier app "+name+" is still being removed (its containers are not gone yet). Retry in a few seconds.")
			return true
		}
	}
	return false
}

// gpuTemplatePreflight refuses a GPU template when the local node has no free GPU,
// naming a remote node that does so the user can pick it. The text carries the
// device_missing compat class so the failure is classified the same way later.
func (rt *Router) gpuTemplatePreflight(ctx context.Context, tpl catalog.Template) string {
	return rt.gpuTemplatePreflightOn(ctx, tpl, nil)
}

// gpuTemplatePreflightOn checks the pinned node's own GPU inventory; a nil target means local.
func (rt *Router) gpuTemplatePreflightOn(ctx context.Context, tpl catalog.Template, target *nodeTarget) string {
	if !tpl.RequiresGPU || rt.models == nil {
		return ""
	}
	nodeID, subject := "", "this node"
	remote := target != nil && target.id != ""
	if remote {
		nodeID, subject = target.id, fmt.Sprintf("node %q", target.name)
	}
	_, known, err := rt.models.NodeGPU(ctx, nodeID)
	if err != nil {
		return ""
	}
	if !known && remote {
		return fmt.Sprintf("template %q needs an NVIDIA GPU (%s) and %s has reported none. Pick a node with a GPU (--node), or wait for its agent to report.", tpl.ID, failure.CompatDeviceMissing, subject)
	}
	if !known {
		return ""
	}
	planner, err := rt.newGPUPlanner(ctx)
	if err != nil || planner == nil {
		return ""
	}
	claim := gpu.Claim{Name: models.ServiceClaimName(tpl.ID), Count: 1}
	localErr := planner.fit(nodeID, claim)
	if localErr == nil {
		return ""
	}
	var other []string
	for id, n := range planner.nodes {
		if id != nodeID && id != "" && n.Eligible() && planner.fit(id, claim) == nil {
			other = append(other, n.Name)
		}
	}
	sort.Strings(other)
	msg := fmt.Sprintf("needs an NVIDIA GPU (%s) and %s cannot give one: %s.", failure.CompatDeviceMissing, subject, localErr)
	if errors.Is(localErr, gpu.ErrRuntimeMissing) {
		msg += " " + gpu.InstallHint
	}
	if len(other) > 0 {
		msg += " Nodes with a free GPU: " + strings.Join(other, ", ") + ". Pick one with --node."
	}
	return fmt.Sprintf("template %q %s", tpl.ID, msg)
}
