package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// applyMigration creates each non-blocking app in report.Apps directly
// on the target Levelrail instance via client.CreateApp (POST
// /api/v1/apps), the same call apps_create.go's own existing-image path
// makes, reusing pendingImageTag for the same reason it does there: no
// build has run yet, but validateAppResource requires a non-empty image
// at creation time.
//
// Secret env vars are never sent as part of the create body:
// appResource has no field for them at all (internal/api/apps.go's own
// appResource, confirmed no SecretEnv/secret_env field), the same gap
// apps_create.go's own planFromFile already documents and works around
// by creating without secrets and setting each one afterward via
// PUT .../secrets/{key}. This mirrors that exact, already-established
// workaround rather than inventing a new one.
func applyMigration(ctx context.Context, client *Client, report *migrationReport) {
	for i := range report.Apps {
		a := &report.Apps[i]
		if a.Blocking {
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

		for key, value := range a.Service.EnvLiteral {
			if err := client.SetSecret(ctx, created.Name, key, value); err != nil {
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
