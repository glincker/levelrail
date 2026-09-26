package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

type errCloneRequest string

func (e errCloneRequest) Error() string { return string(e) }

type cloneDomainOpts struct {
	mode, suffix string
}

func cloneDomainsFor(req cloneAppRequest) (cloneDomainOpts, error) {
	switch req.Domains {
	case "", "none":
		return cloneDomainOpts{mode: "none"}, nil
	case "suffix":
		suffix := strings.ToLower(strings.Trim(strings.TrimSpace(req.DomainSuffix), "-"))
		if suffix == "" || strings.ContainsAny(suffix, ". /") {
			return cloneDomainOpts{}, errors.New("domain_suffix must be a single DNS label")
		}
		return cloneDomainOpts{mode: "suffix", suffix: suffix}, nil
	}
	return cloneDomainOpts{}, errors.New(`domains must be "none" or "suffix"`)
}

// deriveCloneDomains never returns a source domain unchanged, so a clone can
// not shadow its source's ingress.
func deriveCloneDomains(source []string, o cloneDomainOpts) []string {
	if o.mode != "suffix" {
		return nil
	}
	out := make([]string, 0, len(source))
	for _, d := range source {
		label, rest, found := strings.Cut(d, ".")
		if !found || label == "" || label == "*" {
			continue
		}
		out = append(out, label+"-"+o.suffix+"."+rest)
	}
	return out
}

// applyCloneExtras handles the options SaveDesiredService cannot: environment
// placement and re-encrypting secret values under the clone's own slots.
func (rt *Router) applyCloneExtras(ctx context.Context, source store.DesiredService, req cloneAppRequest) error {
	if req.EnvironmentID != "" {
		if err := rt.environments.SetServiceEnvironment(ctx, req.NewName, req.EnvironmentID); err != nil {
			return fmt.Errorf("set environment: %w", err)
		}
	}
	if !req.CopySecrets {
		return nil
	}
	for _, ref := range source.SecretEnv {
		value, err := rt.secrets.Resolve(ctx, source.Name, ref.Name)
		if errors.Is(err, secrets.ErrValueNotFound) {
			continue
		}
		if err != nil {
			return fmt.Errorf("resolve secret %q: %w", ref.Name, err)
		}
		if err := rt.secrets.SetValueGuarded(ctx, req.NewName, ref.Name, value, false); err != nil {
			return fmt.Errorf("copy secret %q: %w", ref.Name, err)
		}
	}
	return nil
}

// validateCloneEnvironment checks envID before anything is written.
func (rt *Router) validateCloneEnvironment(ctx context.Context, source store.DesiredService, envID string) error {
	if envID == "" {
		return nil
	}
	env, err := rt.environments.GetEnvironment(ctx, envID)
	if errors.Is(err, store.ErrEnvironmentNotFound) || (err == nil && env.ProjectID != source.ProjectID) {
		return errCloneRequest("environment_id must be an environment of the source app's project")
	}
	if err != nil {
		return fmt.Errorf("load environment %q: %w", envID, err)
	}
	return nil
}

// rollbackClone removes a partially created clone so a retry does not
// conflict with it.
func (rt *Router) rollbackClone(ctx context.Context, name string) {
	if rt.secrets != nil {
		if err := rt.secrets.DeleteAll(ctx, name); err != nil {
			rt.logger.Error("api: clone app: roll back secrets failed", slog.String("error", err.Error()), slog.String("new_name", name))
		}
	}
	if err := rt.deleteApp(ctx, name); err != nil && !errors.Is(err, store.ErrServiceNotFound) {
		rt.logger.Error("api: clone app: roll back failed", slog.String("error", err.Error()), slog.String("new_name", name))
	}
}

type clonePreviewResource struct {
	Source       string   `json:"source"`
	WillCopy     []string `json:"will_copy"`
	WillNotCopy  []string `json:"will_not_copy"`
	SecretNames  []string `json:"secret_names"`
	SourceDomain []string `json:"source_domains"`
}

// handleClonePreview handles GET /api/v1/apps/{name}/clone/preview: what a
// clone copies and what it leaves behind, without writing anything.
func (rt *Router) handleClonePreview(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	source, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: clone preview: load source failed", err, slog.String("name", name))
		return
	}
	secretNames := make([]string, len(source.SecretEnv))
	for i, s := range source.SecretEnv {
		secretNames[i] = s.Name
	}
	writeJSON(w, http.StatusOK, clonePreviewResource{
		Source: name,
		WillCopy: []string{
			"image, port, command and entrypoint", "plain environment variables", "secret names (values only when copy_secrets is set and you hold read:sensitive)",
			"vault references", "replicas, strategy, resources and health checks", "hooks and egress policy", "volume definitions (as new, empty volumes)", "bind mounts, labels and registry credential",
			"project",
		},
		WillNotCopy: []string{
			"domains (unless domains is suffix, which derives new names)", "volume data", "scheduled tasks", "node placement", "host port pin", "deploy history and metrics",
		},
		SecretNames:  secretNames,
		SourceDomain: source.Domains,
	})
}
