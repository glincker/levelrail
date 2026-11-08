package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	serviceRuntimeBodyLimit = 64 << 10
	codeInvalidRuntime      = "invalid_runtime"
)

// editableRuntimeKeys is what this endpoint may change. Keys outside it
// (shm_size, read_only, cap_drop) stay as the app spec set them.
var editableRuntimeKeys = []string{"dns", "extra_hosts", "init", "pids_limit", "stop_timeout", "sysctls", "tmpfs", "ulimits"}

// neverRuntimeKeys widen the container's reach into the host and are named in the refusal.
var neverRuntimeKeys = map[string]bool{"cap_add": true, "privileged": true, "devices": true, "security_opt": true}

type serviceRuntimeResource struct {
	App      string              `json:"app"`
	Service  string              `json:"service"`
	Revision string              `json:"revision"`
	Runtime  spec.RuntimeOptions `json:"runtime"`
	Editable []string            `json:"editable"`
	Bounds   spec.RuntimeBounds  `json:"bounds"`
}

type serviceRuntimeErrors struct {
	Error  string            `json:"error"`
	Code   string            `json:"code"`
	Errors map[string]string `json:"errors"`
}

// runtimeRevision is a short content hash; any change to the block changes it.
func runtimeRevision(r *spec.RuntimeOptions) string {
	raw, _ := json.Marshal(r)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:16]
}

// serviceRuntimeTarget finds the desired service: the app itself, or a compose member.
func (rt *Router) serviceRuntimeTarget(w http.ResponseWriter, r *http.Request) (*store.DesiredService, string, bool) {
	app, service := r.PathValue("name"), r.PathValue("service")
	target := app
	if service != app {
		target = compose.MemberAppName(app, service)
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), target)
	if err == nil && service != app && !rt.isComposeMember(r.Context(), app, svc) {
		err = store.ErrServiceNotFound
	}
	if err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "service not found")
		} else {
			rt.internalError(w, "api: service runtime: load failed", err, slog.String("app", app), slog.String("service", service))
		}
		return nil, "", false
	}
	return svc, service, true
}

// isComposeMember is true only when the target belongs to the app group named in the path,
// so a name prefix alone never reaches another app.
func (rt *Router) isComposeMember(ctx context.Context, app string, svc *store.DesiredService) bool {
	if svc.AppID == "" || rt.appGroups == nil {
		return false
	}
	group, err := rt.appGroups.GetAppByName(ctx, app)
	return err == nil && group.ID == svc.AppID
}

// runtimeUpdater is the compare-and-swap write; the real store has it.
type runtimeUpdater interface {
	UpdateServiceRuntimeIf(ctx context.Context, name string, expected, next *spec.RuntimeOptions) (bool, error)
}

func serviceRuntimeView(svc *store.DesiredService, app, service string) serviceRuntimeResource {
	res := serviceRuntimeResource{App: app, Service: service, Revision: runtimeRevision(svc.Runtime), Editable: editableRuntimeKeys, Bounds: spec.CurrentRuntimeBounds()}
	if svc.Runtime != nil {
		res.Runtime = *svc.Runtime
	}
	return res
}

// handleGetServiceRuntime handles GET /api/v1/apps/{name}/services/{service}/runtime.
func (rt *Router) handleGetServiceRuntime(w http.ResponseWriter, r *http.Request) {
	svc, service, ok := rt.serviceRuntimeTarget(w, r)
	if !ok {
		return
	}
	res := serviceRuntimeView(svc, r.PathValue("name"), service)
	w.Header().Set("ETag", `"`+res.Revision+`"`)
	writeJSON(w, http.StatusOK, res)
}

// handlePutServiceRuntime handles PUT .../runtime: the body sets the editable keys
// (omitted or null clears them) and If-Match must carry the revision that was read.
func (rt *Router) handlePutServiceRuntime(w http.ResponseWriter, r *http.Request) {
	var raw map[string]json.RawMessage
	if !decodeReqLimit(w, r, &raw, serviceRuntimeBodyLimit) {
		return
	}
	svc, service, ok := rt.serviceRuntimeTarget(w, r)
	if !ok {
		return
	}
	match := strings.Trim(strings.TrimPrefix(strings.TrimSpace(r.Header.Get("If-Match")), "W/"), `"`)
	if match == "" {
		writeError(w, http.StatusPreconditionRequired, "If-Match with the revision from GET is required")
		return
	}
	current := runtimeRevision(svc.Runtime)
	if match != current {
		w.Header().Set("ETag", `"`+current+`"`)
		writeError(w, http.StatusPreconditionFailed, "runtime options changed since you read them, read again and retry")
		return
	}
	next, errs := mergeServiceRuntime(svc, raw)
	if len(errs) > 0 {
		keys := make([]string, 0, len(errs))
		for k := range errs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+": "+errs[k])
		}
		writeJSON(w, http.StatusUnprocessableEntity, serviceRuntimeErrors{Error: strings.Join(parts, "; "), Code: codeInvalidRuntime, Errors: errs})
		return
	}
	upd, canSwap := rt.apps.(runtimeUpdater)
	if !canSwap {
		writeError(w, http.StatusNotImplemented, "runtime options are not available on this store")
		return
	}
	swapped, err := upd.UpdateServiceRuntimeIf(r.Context(), svc.Name, svc.Runtime, next)
	if err != nil {
		rt.internalError(w, "api: service runtime: save failed", err, slog.String("app", svc.Name))
		return
	}
	if !swapped {
		writeError(w, http.StatusPreconditionFailed, "runtime options changed since you read them, read again and retry")
		return
	}
	svc.Runtime = next
	if next != nil {
		store.ApplyRuntimeOptions(svc, next)
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: svc.Name, Kind: store.AppEventConfigChange, Title: "Runtime options changed, the container is recreated", Keys: slices.Sorted(maps.Keys(raw))})
	rt.nudgeReconciler()
	res := serviceRuntimeView(svc, r.PathValue("name"), service)
	w.Header().Set("ETag", `"`+res.Revision+`"`)
	writeJSON(w, http.StatusOK, res)
}

// mergeServiceRuntime builds the new block from the body and
// validates each key on its own so every bad key is reported at once.
func mergeServiceRuntime(svc *store.DesiredService, raw map[string]json.RawMessage) (*spec.RuntimeOptions, map[string]string) {
	errs := map[string]string{}
	// PUT replaces the editable keys; the ones the spec owns carry over.
	next := spec.RuntimeOptions{}
	if svc.Runtime != nil {
		next = spec.RuntimeOptions{ShmSize: svc.Runtime.ShmSize, ReadOnly: svc.Runtime.ReadOnly, CapDrop: svc.Runtime.CapDrop}
	}
	for key, val := range raw {
		if !slices.Contains(editableRuntimeKeys, key) {
			if neverRuntimeKeys[key] {
				errs[key] = "never allowed, it widens the container's access to the host"
			} else {
				errs[key] = "not editable here, allowed keys: " + strings.Join(editableRuntimeKeys, ", ")
			}
			continue
		}
		one := spec.RuntimeOptions{}
		if err := setRuntimeKey(&one, key, val); err != nil {
			errs[key] = "wrong type: " + err.Error()
			continue
		}
		if err := one.Validate(); err != nil {
			errs[key] = strings.TrimPrefix(err.Error(), "runtime.")
			continue
		}
		if err := setRuntimeKey(&next, key, val); err != nil {
			errs[key] = "wrong type: " + err.Error()
		}
	}
	if _, set := raw["sysctls"]; set && len(next.Sysctls) > 0 && svc.Egress != nil && errs["sysctls"] == "" {
		errs["sysctls"] = "cannot be combined with egress, which shares the network namespace"
	}
	if len(errs) > 0 {
		return nil, errs
	}
	if err := next.Validate(); err != nil {
		return nil, map[string]string{"runtime": strings.TrimPrefix(err.Error(), "runtime.")}
	}
	if blank, _ := json.Marshal(next); string(blank) == "{}" {
		return nil, nil
	}
	return &next, nil
}

// setRuntimeKey decodes val strictly into the one field; null clears it.
func setRuntimeKey(o *spec.RuntimeOptions, key string, val json.RawMessage) error {
	var dst any
	switch key {
	case "dns":
		o.DNS, dst = nil, &o.DNS
	case "extra_hosts":
		o.ExtraHosts, dst = nil, &o.ExtraHosts
	case "init":
		o.Init, dst = nil, &o.Init
	case "pids_limit":
		o.PidsLimit, dst = 0, &o.PidsLimit
	case "stop_timeout":
		o.StopTimeout, dst = nil, &o.StopTimeout
	case "sysctls":
		o.Sysctls, dst = nil, &o.Sysctls
	case "tmpfs":
		o.Tmpfs, dst = nil, &o.Tmpfs
	case "ulimits":
		o.Ulimits, dst = nil, &o.Ulimits
	default:
		return fmt.Errorf("unknown key")
	}
	dec := json.NewDecoder(bytes.NewReader(val))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.New("value does not match the expected shape")
	}
	return nil
}
