package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// Secrets cannot go in the create body (appResource has no field for them), so they are set afterward.
// validateAppResource needs a non-empty image at create time, so pendingImageTag stands in before any build.
func applyMigration(ctx context.Context, client *Client, report *migrationReport) {
	for i := range report.Apps {
		a := &report.Apps[i]
		if a.Blocking {
			continue
		}
		if a.Compose != nil {
			// Staged by the pipeline before apply, since staging writes files only.
			continue
		}

		body := appResource{
			Name:    a.ServiceName,
			Image:   pendingImageTag(a.ServiceName),
			Port:    a.Service.Port,
			Domains: a.Service.Domains,
		}
		if a.Service.Memory != "" || a.Service.CPU > 0 {
			if res, err := toServiceResources(&spec.Resources{Memory: a.Service.Memory, CPU: a.Service.CPU}); err == nil {
				body.Resources = res
			}
		}
		if a.Service.Health != nil {
			if h, err := toServiceHealth(healthMappedToSpec(*a.Service.Health)); err == nil {
				body.Health = h
			}
		}

		created, err := client.CreateApp(ctx, body)
		if err != nil {
			a.CreateError = fmt.Errorf("create app %q: %w", a.ServiceName, err).Error()
			continue
		}
		a.Applied = true

		if pv := a.Service.Previews; pv != nil {
			req := apiclient.SetPreviewPolicyRequest{}
			if pv.MaxPreviews > 0 {
				req.MaxPreviews = &pv.MaxPreviews
			}
			if _, err := client.SetPreviewPolicy(ctx, created.Name, req); err != nil {
				a.Issues = append(a.Issues, migrationIssue{Field: "previews", Severity: issueReview, Detail: "preview policy was not applied: " + err.Error()})
			}
		}

		for key, value := range a.Service.EnvLiteral {
			if err := client.SetSecret(ctx, created.Name, key, value, false); err != nil {
				a.SecretsFailed = append(a.SecretsFailed, key)
				continue
			}
			a.SecretsSet = append(a.SecretsSet, key)
		}
		sort.Strings(a.SecretsSet)
		sort.Strings(a.SecretsFailed)
	}
}

func healthMappedToSpec(h mappedHealth) *spec.Health {
	probe := spec.Probe{Path: h.Path, Failures: h.Failures}
	if h.Interval > 0 {
		probe.Interval = fmt.Sprintf("%ds", h.Interval)
	}
	if h.Timeout > 0 {
		probe.Timeout = fmt.Sprintf("%ds", h.Timeout)
	}
	return &spec.Health{Readiness: &probe, Liveness: &probe}
}

// defaultStageDir is where staged compose apps go. Staging writes files only,
// because the compose deploy route starts containers.
const defaultStageDir = "./migrated/compose"

// stageComposeApp writes the translated compose file with owner-only
// permissions (it holds generated secrets) and records the deploy command.
func stageComposeApp(a *mappedApp, dir string) {
	if dir == "" {
		dir = defaultStageDir
	}
	if hasRawMagicVar(a.Compose.Yaml) {
		a.CreateError = "unresolved SERVICE_ variables remain, fix them in the staged file before deploying"
	}
	appDir := filepath.Join(dir, a.ServiceName)
	if err := os.MkdirAll(appDir, 0o700); err != nil { //nolint:gosec // name is sanitized, dir is operator-supplied
		a.CreateError = fmt.Errorf("stage compose app %q: %w", a.ServiceName, err).Error()
		return
	}
	path := filepath.Join(appDir, "compose.yaml")
	if err := os.WriteFile(path, []byte(a.Compose.Yaml), 0o600); err != nil { //nolint:gosec // name is sanitized, dir is operator-supplied
		a.CreateError = fmt.Errorf("stage compose app %q: %w", a.ServiceName, err).Error()
		return
	}
	a.Compose.StagedPath = path
	a.Compose.Deploy = fmt.Sprintf("levelrail-cli compose deploy %s -f %s", a.ServiceName, path)
	a.Applied = a.CreateError == ""
}
