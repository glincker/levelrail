package deploy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// MultiRequest is one deploy attempt for N services under one app,
// fanned out from a single app.yaml's Spec.Services map. Mirrors
// Request's own fields, minus ServiceName/Service/ImageRepo (one per
// service key instead of one for the whole request) and plus AppName
// and ImageRepoBase.
type MultiRequest struct {
	// AppName identifies the store.App every fanned-out service is
	// linked to (created if it doesn't exist yet, reused if it does: see
	// DeploySpec's own doc comment). Each service's own DesiredService
	// name is "<AppName>-<serviceKey>", the flat naming convention
	// internal/api/apps_group.go's own doc comment establishes.
	AppName string
	// Services is app.yaml's services: map, keyed by service name (e.g.
	// "web", "worker").
	Services map[string]spec.Service
	// SourceDir is the local checkout root, shared by every service's
	// build: one git checkout produces every service's image.
	SourceDir string
	CommitSHA string
	// ImageRepoBase is the image name without a tag or per-service
	// suffix, e.g. "levelrail/myapp"; each service tags as
	// "<ImageRepoBase>-<serviceKey>:<CommitSHA>".
	ImageRepoBase string
}

// ServiceOutcome is one service's own fan-out result: Image is set on
// success, Err is set on failure, never both.
type ServiceOutcome struct {
	ServiceKey  string
	ServiceName string
	Image       string
	Err         error
}

// ErrNoServices is returned by DeploySpec when req.Services is empty:
// there is nothing to fan out, and proceeding would create an App row
// that owns nothing.
var ErrNoServices = errors.New("deploy: no services to deploy")

// DeploySpec fans out req into N independent Deploy calls, one per
// service key, under one store.App (created if AppName has none yet,
// reused otherwise: p.apps.GetAppByName then SaveApp, App.ID == App.Name
// by this method's own convention, matching migrations/0039_apps.sql's
// backfill convention for a pre-existing single-service app). The app
// row is created before any service's build starts, so a partially
// failed fan-out still leaves every successfully-deployed sibling
// linked to a real AppID.
//
// Each service key's build failure is captured in that key's own
// ServiceOutcome.Err and does not block any other key's build or
// deploy, matching the reconciler's own "one broken resource must not
// block convergence of everything else" principle
// (internal/reconcile/engine.go's ReconcileAll doc comment): a caller
// gets back one outcome per service key, in deterministic (sorted key)
// order, and must inspect each Err individually rather than treating
// this call's own returned error as "did it work." The returned error
// is non-nil only for a failure that blocks every service alike
// (req.Services empty, or the app row itself couldn't be created or
// reused); once fan-out begins, DeploySpec itself always returns a nil
// error and lets each ServiceOutcome carry its own.
//
// progress, if non-nil, is called with each service key's own
// build.ProgressEvent stream, so a caller can tell which service a
// given progress line belongs to.
func (p *Pipeline) DeploySpec(ctx context.Context, req MultiRequest, progress func(serviceKey string, ev build.ProgressEvent)) ([]ServiceOutcome, error) {
	if len(req.Services) == 0 {
		return nil, fmt.Errorf("deploy: app %q: %w", req.AppName, ErrNoServices)
	}
	if p.apps == nil {
		return nil, fmt.Errorf("deploy: app %q: no app store configured for a multi-service deploy (see WithAppStore)", req.AppName)
	}

	services, healthWarnings, err := expandComposeServices(req.Services, req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("deploy: app %q: %w", req.AppName, err)
	}
	req.Services = services
	for _, warning := range healthWarnings {
		p.logger.Warn("deploy: compose healthcheck not translatable to a readiness probe", slog.String("app", req.AppName), slog.String("detail", warning))
	}

	appID, err := p.ensureApp(ctx, req.AppName)
	if err != nil {
		return nil, fmt.Errorf("deploy: app %q: %w", req.AppName, err)
	}

	keys := make([]string, 0, len(req.Services))
	for k := range req.Services {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	outcomes := make([]ServiceOutcome, 0, len(keys))
	for _, key := range keys {
		svcName := req.AppName + "-" + key

		var svcProgress func(build.ProgressEvent)
		if progress != nil {
			svcProgress = func(ev build.ProgressEvent) { progress(key, ev) }
		}

		image, deployErr := p.Deploy(ctx, Request{
			ServiceName: svcName,
			Service:     req.Services[key],
			SourceDir:   req.SourceDir,
			CommitSHA:   req.CommitSHA,
			ImageRepo:   req.ImageRepoBase + "-" + key,
		}, svcProgress)
		if deployErr != nil {
			outcomes = append(outcomes, ServiceOutcome{ServiceKey: key, ServiceName: svcName, Err: deployErr})
			continue
		}

		if linkErr := p.apps.UpdateServiceApp(ctx, svcName, appID); linkErr != nil {
			outcomes = append(outcomes, ServiceOutcome{ServiceKey: key, ServiceName: svcName, Err: fmt.Errorf("link service to app: %w", linkErr)})
			continue
		}

		outcomes = append(outcomes, ServiceOutcome{ServiceKey: key, ServiceName: svcName, Image: image})
	}
	return outcomes, nil
}

// expandComposeServices replaces every build.type: compose entry in
// services with the real services its referenced compose file declares
// (compose.ExpandBuildService), so DeploySpec's own fan-out loop below
// never needs to know compose exists: every entry it sees by the time
// it runs is an ordinary dockerfile/image/railpack/static service. A
// service with any other build.type passes through unchanged.
//
// Returns an error, rather than silently dropping or overwriting, on a
// key collision between an expanded compose service and any other
// entry (compose-expanded or not): silently picking one would deploy
// the wrong thing under a name the operator didn't expect.
func expandComposeServices(services map[string]spec.Service, sourceDir string) (out map[string]spec.Service, warnings []string, err error) {
	hasCompose := false
	for _, svc := range services {
		if svc.Build.Type == spec.BuildCompose {
			hasCompose = true
			break
		}
	}
	if !hasCompose {
		return services, nil, nil
	}

	out = make(map[string]spec.Service, len(services))
	for key, svc := range services {
		if svc.Build.Type != spec.BuildCompose {
			if _, exists := out[key]; exists {
				return nil, nil, fmt.Errorf("service %q collides with a service another compose file already declared", key)
			}
			out[key] = svc
			continue
		}

		expanded, expandedWarnings, err := compose.ExpandBuildService(svc, sourceDir)
		if err != nil {
			return nil, nil, fmt.Errorf("service %q: %w", key, err)
		}
		warnings = append(warnings, expandedWarnings...)
		for expandedKey, expandedSvc := range expanded {
			if _, exists := out[expandedKey]; exists {
				return nil, nil, fmt.Errorf("service %q: compose service %q collides with another service of the same name", key, expandedKey)
			}
			out[expandedKey] = expandedSvc
		}
	}
	return out, warnings, nil
}

// ensureApp returns name's store.App ID, creating the row (ID == Name)
// if it doesn't exist yet. name is a service or app name, never a
// user-facing display label, so it's safe to use directly as the ID:
// the same convention migrations/0039_apps.sql's own backfill already
// establishes for every pre-existing single-service app.
func (p *Pipeline) ensureApp(ctx context.Context, name string) (string, error) {
	existing, err := p.apps.GetAppByName(ctx, name)
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, store.ErrAppNotFound) {
		return "", fmt.Errorf("look up app: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	app := store.App{ID: name, Name: name, CreatedAt: now, UpdatedAt: now}
	if err := p.apps.SaveApp(ctx, app); err != nil {
		return "", fmt.Errorf("save app: %w", err)
	}
	return app.ID, nil
}
