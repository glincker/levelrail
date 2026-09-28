package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// promoteImageRef is the seam that picks what a promotion deploys. Today it is
// the source's image reference; a content digest can replace it here later.
func promoteImageRef(source store.DesiredService) string {
	return source.Image
}

// promoteDiff describes what promoting source onto target would change. Env
// values are never included, only key names.
type promoteDiff struct {
	Image         *deployCompareField  `json:"image,omitempty"`
	Replicas      *deployCompareField  `json:"replicas,omitempty"`
	Resources     *deployCompareField  `json:"resources,omitempty"`
	Health        *deployCompareField  `json:"health,omitempty"`
	EnvAdded      []string             `json:"env_added"`
	EnvRemoved    []string             `json:"env_removed"`
	EnvChanged    []string             `json:"env_changed"`
	SecretsAdded  []string             `json:"secret_keys_added"`
	SecretsGone   []string             `json:"secret_keys_removed"`
	Untouched     []string             `json:"untouched"`
	Others        []deployCompareField `json:"other_differences,omitempty"`
	AppliedByFlag string               `json:"env_applied_with,omitempty"`
}

func buildPromoteDiff(source, target store.DesiredService) promoteDiff {
	d := promoteDiff{
		EnvAdded: []string{}, EnvRemoved: []string{}, EnvChanged: []string{},
		SecretsAdded: []string{}, SecretsGone: []string{},
		Untouched:     []string{"domains", "port", "node placement", "volumes", "secret values", "environment values that already exist on the target"},
		AppliedByFlag: "include_env",
	}
	if ref := promoteImageRef(source); ref != target.Image {
		d.Image = &deployCompareField{Field: "image", From: target.Image, To: ref}
	}
	if source.Replicas != target.Replicas {
		d.Replicas = &deployCompareField{Field: "replicas", From: fmt.Sprint(target.Replicas), To: fmt.Sprint(source.Replicas)}
	}
	if a, b := fmt.Sprintf("%+v", target.Resources), fmt.Sprintf("%+v", source.Resources); a != b {
		d.Resources = &deployCompareField{Field: "resources", From: a, To: b}
	}
	if a, b := fmt.Sprintf("%+v", target.Health), fmt.Sprintf("%+v", source.Health); a != b {
		d.Health = &deployCompareField{Field: "health", From: a, To: b}
	}
	for k, v := range source.Env {
		tv, ok := target.Env[k]
		switch {
		case !ok:
			d.EnvAdded = append(d.EnvAdded, k)
		case tv != v:
			d.EnvChanged = append(d.EnvChanged, k)
		}
	}
	for k := range target.Env {
		if _, ok := source.Env[k]; !ok {
			d.EnvRemoved = append(d.EnvRemoved, k)
		}
	}
	srcSecrets, dstSecrets := secretNameSet(source), secretNameSet(target)
	for k := range srcSecrets {
		if !dstSecrets[k] {
			d.SecretsAdded = append(d.SecretsAdded, k)
		}
	}
	for k := range dstSecrets {
		if !srcSecrets[k] {
			d.SecretsGone = append(d.SecretsGone, k)
		}
	}
	for _, s := range [][]string{d.EnvAdded, d.EnvRemoved, d.EnvChanged, d.SecretsAdded, d.SecretsGone} {
		sort.Strings(s)
	}
	return d
}

func secretNameSet(s store.DesiredService) map[string]bool {
	out := make(map[string]bool, len(s.SecretEnv))
	for _, r := range s.SecretEnv {
		out[r.Name] = true
	}
	return out
}

// applyPromoteEnv adds the source's new plain env keys to target and removes
// keys the source no longer has. Keys present on both sides keep the target's
// value, since values legitimately differ between environments.
func applyPromoteEnv(target *store.DesiredService, source store.DesiredService, d promoteDiff) {
	env := make(map[string]string, len(target.Env))
	for k, v := range target.Env {
		if !slices.Contains(d.EnvRemoved, k) {
			env[k] = v
		}
	}
	for _, k := range d.EnvAdded {
		env[k] = source.Env[k]
	}
	target.Env = env
}

// promoteBlockers lists reasons the source is not fit to promote: an attention
// condition or a failed latest deploy. A source with no status yet is allowed.
func (rt *Router) promoteBlockers(ctx context.Context, source store.DesiredService) ([]string, error) {
	var blockers []string
	conds, err := rt.deploys.GetConditionsForControllers(ctx, []string{applicationControllerName(source.Name)})
	if err != nil {
		return nil, fmt.Errorf("load source conditions: %w", err)
	}
	if summarizeAppConditions(conds[applicationControllerName(source.Name)]).Label == "Attention needed" {
		blockers = append(blockers, "source app is not healthy")
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, source.Name)
	if err != nil {
		return nil, fmt.Errorf("load source deploy attempts: %w", err)
	}
	if len(attempts) > 0 && attempts[0].Status == store.DeployAttemptStatusFailed {
		blockers = append(blockers, "source app's last deploy failed")
	}
	return blockers, nil
}

// promoteNeedsConfirmation is true for a protected environment and for one
// named production, unless the caller already confirmed.
func promoteNeedsConfirmation(env store.Environment, confirm bool) bool {
	return !confirm && (env.Protected || strings.EqualFold(env.Name, "production"))
}

// recordPromoteAudit writes an audit row on each side of a promotion, since
// the route-level audit only records the source path.
func (rt *Router) recordPromoteAudit(ctx context.Context, r *http.Request, source, target string) {
	p, err := rt.bulkPrincipal(r)
	if err != nil {
		rt.logger.Warn("api: promote audit principal failed", slog.String("error", err.Error()))
		return
	}
	rt.recordBulkAudit(ctx, r, p, AbilityDeploy, "promote-from", source)
	rt.recordBulkAudit(ctx, r, p, AbilityDeploy, "promote-to", target)
}
