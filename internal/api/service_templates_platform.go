package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/failure"
	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/models"
	"github.com/GLINCKER/levelrail/internal/store"
)

// platformPreflight compares a template's verified platforms and GPU need with
// the host that will run it. It returns a plain refusal, or "" when the install can go on.
// overridable is true only for an arch mismatch, which emulation may still serve.
func (rt *Router) platformPreflight(ctx context.Context, tpl catalog.Template) (reason string, overridable bool) {
	host := rt.containerPlatform(ctx)
	if len(tpl.Platforms) > 0 && !containsString(tpl.Platforms, host) {
		return fmt.Sprintf("template %q has no %s build (verified: %s). Pick a node with a matching architecture, or pin another image tag in the template settings. To try it anyway under emulation, repeat with allow_platform_mismatch.", tpl.ID, host, strings.Join(tpl.Platforms, ", ")), true
	}
	if reason := rt.gpuTemplatePreflight(ctx, tpl); reason != "" {
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
	if !tpl.RequiresGPU || rt.models == nil {
		return ""
	}
	if _, known, err := rt.models.NodeGPU(ctx, ""); err != nil || !known {
		return ""
	}
	planner, err := rt.newGPUPlanner(ctx)
	if err != nil || planner == nil {
		return ""
	}
	claim := gpu.Claim{Name: models.ServiceClaimName(tpl.ID), Count: 1}
	localErr := planner.fit("", claim)
	if localErr == nil {
		return ""
	}
	var other []string
	for id, n := range planner.nodes {
		if id != "" && n.Eligible() && planner.fit(id, claim) == nil {
			other = append(other, n.Name)
		}
	}
	sort.Strings(other)
	msg := fmt.Sprintf("needs an NVIDIA GPU (%s) and this node cannot give one: %s.", failure.CompatDeviceMissing, localErr)
	if errors.Is(localErr, gpu.ErrRuntimeMissing) {
		msg += " " + gpu.InstallHint
	}
	if len(other) > 0 {
		msg += " Nodes with a free GPU: " + strings.Join(other, ", ") + "."
	}
	return fmt.Sprintf("template %q %s", tpl.ID, msg)
}
