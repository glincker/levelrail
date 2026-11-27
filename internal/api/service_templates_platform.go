package api

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"strings"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/store"
)

// platformPreflight compares a template's verified platforms and GPU need with
// the host that will run it. It returns a plain refusal, or "" when the install can go on.
// overridable is true only for an arch mismatch, which emulation may still serve.
func (rt *Router) platformPreflight(ctx context.Context, tpl catalog.Template) (reason string, overridable bool) {
	host := runtime.GOOS + "/" + runtime.GOARCH
	if len(tpl.Platforms) > 0 && !containsString(tpl.Platforms, host) {
		return fmt.Sprintf("template %q has no %s build (verified: %s). Pick a node with a matching architecture, or pin another image tag in the template settings. To try it anyway under emulation, repeat with allow_platform_mismatch.", tpl.ID, host, strings.Join(tpl.Platforms, ", ")), true
	}
	if tpl.RequiresGPU && rt.models != nil {
		info, known, err := rt.models.NodeGPU(ctx, "")
		if err == nil && known && (!info.Present || !info.RuntimeInstalled) {
			if !info.Present {
				return fmt.Sprintf("template %q needs an NVIDIA GPU and this node reports none. Install it on a GPU node.", tpl.ID), false
			}
			return fmt.Sprintf("template %q needs an NVIDIA GPU and this node has no nvidia container runtime. Install the NVIDIA container toolkit, then retry.", tpl.ID), false
		}
	}
	if reason := kernelModulePreflight(tpl); reason != "" {
		return reason, false
	}
	return "", false
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
