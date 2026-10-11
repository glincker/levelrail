package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/scheduledtask"
	"github.com/GLINCKER/levelrail/internal/store"
)

// jobSpecBuilder derives a scheduled job's container from the service's own
// reconcile options, so a job sees exactly the env (secrets, vault, shared
// env, database URLs), volumes and network the service itself gets.
type jobSpecBuilder struct {
	deps dynamicSourceDeps
}

var _ scheduledtask.SpecBuilder = jobSpecBuilder{}

func newJobSpecBuilder(deps dynamicSourceDeps) jobSpecBuilder {
	return jobSpecBuilder{deps: deps}
}

func (b jobSpecBuilder) controller(svc *store.DesiredService, rt docker.Runtime) *application.Controller {
	return application.New(svc.Name, b.deps.db, rt, appOptionsFor(b.deps)...)
}

// BuildJobSpec implements scheduledtask.SpecBuilder.
func (b jobSpecBuilder) BuildJobSpec(ctx context.Context, svc *store.DesiredService, rt docker.Runtime, name string, command []string, res *docker.Resources) (application.JobSpec, error) {
	spec, err := b.controller(svc, rt).BuildJobSpec(ctx, svc, name, command, res)
	if err != nil {
		return application.JobSpec{}, fmt.Errorf("job spec for %q: %w", svc.Name, err)
	}
	return spec, nil
}

// JobSecretValues implements scheduledtask.SpecBuilder.
func (b jobSpecBuilder) JobSecretValues(ctx context.Context, svc *store.DesiredService, rt docker.Runtime) ([]string, error) {
	values, err := b.controller(svc, rt).JobSecretValues(ctx, svc)
	if err != nil {
		return nil, fmt.Errorf("job secrets for %q: %w", svc.Name, err)
	}
	return values, nil
}

// jobsPerAppCap is the runner's per-app job cap, 0 (the API's own default)
// when no runner is wired.
func jobsPerAppCap(r *scheduledtask.Runner) int {
	if r == nil {
		return 0
	}
	return r.Limits.MaxPerApp
}
