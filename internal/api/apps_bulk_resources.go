package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"

	"github.com/GLINCKER/levelrail/internal/importplan"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	bulkActionSetMemory = "set-memory"
	bulkActionSetCPU    = "set-cpu"
	bulkActionSetEnv    = "set-env"
)

func isBulkResourceAction(action string) bool {
	return action == bulkActionSetMemory || action == bulkActionSetCPU || action == bulkActionSetEnv
}

// validateBulkResources checks the request shape once; per-app rules (quota,
// compose limits, secret-backed keys) are applied to each app.
func validateBulkResources(req bulkAppsRequest) error {
	switch req.Action {
	case bulkActionSetMemory:
		if req.MemoryBytes == nil || *req.MemoryBytes <= 0 {
			return errors.New("set-memory needs memory_bytes above zero")
		}
	case bulkActionSetCPU:
		if req.NanoCPUs == nil || *req.NanoCPUs <= 0 {
			return errors.New("set-cpu needs nano_cpus above zero")
		}
	case bulkActionSetEnv:
		if !bundleEnvKeyRe.MatchString(req.EnvKey) {
			return errors.New("set-env needs env_key made of letters, digits and underscores, not starting with a digit")
		}
		if req.EnvValue == nil {
			return errors.New("set-env needs env_value")
		}
	}
	return nil
}

// bulkResources plans one app's memory, CPU or env change and applies it unless
// dry. A raise is checked against quota the same way the single-app edit is, so a
// dry run reports the refusal an apply would hit.
func (rt *Router) bulkResources(ctx context.Context, req bulkAppsRequest, svc store.DesiredService, dry bool, actor string) (status, msg, before, after string) {
	next := svc
	next.Env = maps.Clone(svc.Env)
	if svc.Resources != nil {
		res := *svc.Resources
		next.Resources = &res
	}
	switch req.Action {
	case bulkActionSetMemory, bulkActionSetCPU:
		if next.Resources == nil {
			next.Resources = &store.ServiceResources{}
		}
		if req.Action == bulkActionSetMemory {
			before, after = formatBulkMemory(next.Resources.MemoryBytes), formatBulkMemory(*req.MemoryBytes)
			next.Resources.MemoryBytes = *req.MemoryBytes
		} else {
			before, after = formatBulkCPU(next.Resources.NanoCPUs), formatBulkCPU(*req.NanoCPUs)
			next.Resources.NanoCPUs = *req.NanoCPUs
		}
		if before == after {
			return bulkStatusSkipped, "already " + after, before, after
		}
		if err := validateServiceResources(next.Resources); err != nil {
			return bulkStatusSkipped, err.Error(), before, after
		}
	case bulkActionSetEnv:
		if bulkEnvKeyIsReference(svc, req.EnvKey) {
			return bulkStatusSkipped, req.EnvKey + " is set from a secret, vault or database reference", "", ""
		}
		old, had := svc.Env[req.EnvKey]
		before = "(unset)"
		if had {
			before = old
		}
		after = *req.EnvValue
		if had && old == after {
			return bulkStatusSkipped, "already set to that value", bulkMaskEnv(req.EnvKey, before), bulkMaskEnv(req.EnvKey, after)
		}
		// Results and the dashboard table must not echo secret-looking values.
		defer func() { before, after = bulkMaskEnv(req.EnvKey, before), bulkMaskEnv(req.EnvKey, after) }()
		if next.Env == nil {
			next.Env = map[string]string{}
		}
		next.Env[req.EnvKey] = after
		next.EnvDirty = true
	}
	if req.Action != bulkActionSetEnv {
		release, denied := rt.bulkResizeQuota(ctx, svc, next)
		if release == nil {
			return bulkStatusSkipped, denied, before, after
		}
		defer release()
	}
	if dry {
		return bulkStatusOK, "", before, after
	}
	if err := rt.apps.SaveDesiredService(ctx, next); err != nil {
		rt.logger.Error("api: bulk apps: save failed", slog.String("error", err.Error()), slog.String("name", svc.Name), slog.String("action", req.Action))
		return bulkStatusError, req.Action + " failed", before, after
	}
	rt.nudgeReconciler()
	if req.Action == bulkActionSetEnv {
		rt.snapshotEnvRevision(ctx, svc.Name, actor, next.Env)
		if rt.envProtect.changes != nil {
			if changes := diffEnvKeys(svc.Env, next.Env); len(changes) > 0 {
				if err := rt.envProtect.changes.RecordEnvChanges(ctx, svc.Name, actor, "bulk", changes); err != nil {
					rt.logger.Warn("api: record env changes failed", slog.String("name", svc.Name))
				}
			}
		}
		return bulkStatusOK, "", before, after
	}
	return bulkStatusOK, before + " to " + after, before, after
}

// bulkMaskEnv hides the value of a secret-looking variable in bulk results.
func bulkMaskEnv(key, v string) string {
	if v == "(unset)" || !importplan.KeyLooksSecret(key) {
		return v
	}
	return "(hidden)"
}

func bulkEnvKeyIsReference(svc store.DesiredService, key string) bool {
	for _, ref := range svc.SecretEnv {
		if ref.Name == key {
			return true
		}
	}
	_, vault := svc.VaultEnv[key]
	_, db := svc.DatabaseEnv[key]
	return vault || db
}

func formatBulkMemory(n int64) string {
	const mib = 1 << 20
	switch {
	case n <= 0:
		return "unlimited"
	case n%(1<<30) == 0:
		return fmt.Sprintf("%d GiB", n>>30)
	default:
		return fmt.Sprintf("%d MiB", n/mib)
	}
}

func formatBulkCPU(n int64) string {
	if n <= 0 {
		return "unlimited"
	}
	return fmt.Sprintf("%g CPU", float64(n)/nanoCPUsPerCore)
}
