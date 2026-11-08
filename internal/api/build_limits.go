package api

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// projectBuildFull reports whether the project that owns app already runs as many deploys as the
// per-project limit allows. Apps outside any project and a limit of 0 are never held back.
func (rt *Router) projectBuildFull(ctx context.Context, app string, runningApps []string) bool {
	limit := rt.runtimeSettings.Limit(ctx, runtimesettings.KeyDeployMaxConcurrentPerProject)
	if limit <= 0 || rt.apps == nil {
		return false
	}
	project := rt.serviceProject(ctx, app)
	if project == "" {
		return false
	}
	n := 0
	for _, name := range runningApps {
		if rt.serviceProject(ctx, name) == project {
			n++
		}
	}
	return n >= limit
}

func (rt *Router) serviceProject(ctx context.Context, name string) string {
	svc, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil || svc == nil {
		return ""
	}
	return svc.ProjectID
}

// runningApps lists the app of every running or starting deploy, one entry per deploy.
func (rt *Router) runningApps(running []string) []string {
	out := append([]string(nil), running...)
	for name, n := range rt.startingDeploys {
		for i := 0; i < n; i++ {
			out = append(out, name)
		}
	}
	return out
}
