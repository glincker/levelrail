package api

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploytrace"
	"github.com/GLINCKER/levelrail/internal/store"
)

// traceInstantDeploy writes the pre-reconcile steps of an attempt that has
// no build: how the image was resolved and what config the node is asked to
// run. The reconciler adds the pull, create, probe and cutover steps later.
func (rt *Router) traceInstantDeploy(ctx context.Context, id string, svc store.DesiredService, image, source string, res deploy.ImageResolution, pre *deploytrace.Tracer) {
	sink, ok := rt.deployAttempts.(deploytrace.Sink)
	if !ok {
		return
	}
	tr := pre.Clone()
	if tr == nil {
		tr = deploytrace.NewBuffered()
	}
	if !tr.Has(deploytrace.StepResolveDigest) {
		switch {
		case res.Digest != "":
			tr.Note(ctx, deploytrace.StepResolveDigest, store.TraceStatusDone, res.Reason, fmt.Sprintf("%s pinned to %s", image, res.Digest))
		default:
			tr.Note(ctx, deploytrace.StepResolveDigest, store.TraceStatusSkipped, "NotResolved",
				fmt.Sprintf("%s deploy does not resolve a digest, the node pulls %s as given", source, image))
		}
	}
	deploytrace.ConfigStep(ctx, tr, svc)
	tr.Flush(ctx, sink, id, svc.Name)
}

// composeStartOrder returns service keys with dependencies first, ties broken
// by name so the plan is stable. A cycle falls back to name order; the compose
// validator already rejects cycles before this runs.
func composeStartOrder(appName string, services []store.DesiredService) []string {
	deps := map[string][]string{}
	var keys []string
	for _, s := range services {
		k := strings.TrimPrefix(s.Name, appName+"-")
		keys = append(keys, k)
		deps[k] = s.DependsOn
	}
	sort.Strings(keys)
	placed := map[string]bool{}
	var out []string
	for len(out) < len(keys) {
		progressed := false
		for _, k := range keys {
			if placed[k] {
				continue
			}
			ready := true
			for _, d := range deps[k] {
				if _, known := deps[d]; known && !placed[d] {
					ready = false
				}
			}
			if ready {
				placed[k] = true
				out = append(out, k)
				progressed = true
			}
		}
		if !progressed {
			for _, k := range keys {
				if !placed[k] {
					out = append(out, k)
				}
			}
			break
		}
	}
	return out
}

// traceComposePlan records the per-service plan and start order, and returns
// the tracer to hand each service's attempt (clone it per service).
func traceComposePlan(ctx context.Context, tr *deploytrace.Tracer, appName string, services []store.DesiredService) {
	order := composeStartOrder(appName, services)
	lines := make([]string, 0, len(services))
	byKey := map[string]store.DesiredService{}
	for _, s := range services {
		byKey[strings.TrimPrefix(s.Name, appName+"-")] = s
	}
	for _, k := range order {
		lines = append(lines, k+": "+composeServiceLine(byKey[k]))
	}
	tr.Note(ctx, deploytrace.StepComposePlan, store.TraceStatusDone, "",
		fmt.Sprintf("%d service(s), start order %s; %s", len(services), strings.Join(order, " -> "), strings.Join(lines, "; ")))
}

func composeServiceLine(s store.DesiredService) string {
	line := s.Image
	if s.Port != 0 {
		line += fmt.Sprintf(", port %d", s.Port)
	}
	healthy := map[string]bool{}
	for _, d := range s.DependsOnHealthy {
		healthy[d] = true
	}
	for _, d := range s.DependsOn {
		cond := "started"
		if healthy[d] {
			cond = "healthy"
		}
		line += fmt.Sprintf(", waits for %s (%s)", d, cond)
	}
	return line
}
