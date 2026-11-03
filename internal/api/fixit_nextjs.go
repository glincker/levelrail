package api

import (
	"context"
	"strings"

	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/store"
)

// nextjsMinMemoryMB is the limit under which a Next.js server risks being OOM-killed.
const nextjsMinMemoryMB = 256

// nextjsFixes registers the Next.js doctor fixes for apps whose last deploy
// detected Next.js. Other apps cost one list call at startup and nothing after.
func (rt *Router) nextjsFixes() []fixit.Fix {
	if rt.apps == nil || rt.deployAttempts == nil {
		return nil
	}
	ctx := context.Background()
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil
	}
	deps := fixit.NextjsDeps{Facts: rt.nextjsFacts}
	var out []fixit.Fix
	for _, s := range svcs {
		if rt.appIsNextjs(ctx, s.Name) {
			out = append(out, fixit.NextjsFixes(s.Name, deps)...)
		}
	}
	return out
}

func (rt *Router) appIsNextjs(ctx context.Context, name string) bool {
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
	if err != nil {
		return false
	}
	for _, a := range attempts {
		if a.DetectedFramework != "" {
			return strings.EqualFold(a.DetectedFramework, "Next.js")
		}
	}
	return false
}

// nextjsFacts reports what the stored spec shows. Build output facts are not
// kept after a build, so they read as passing rather than raising false alarms.
func (rt *Router) nextjsFacts(ctx context.Context, app string) (fixit.NextjsFacts, bool, error) {
	svc, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil || svc == nil {
		return fixit.NextjsFacts{}, false, err
	}
	f := fixit.NextjsFacts{
		App: app, Standalone: true, StaticCopied: true, SharpAvailable: true,
		MinMemoryMB: nextjsMinMemoryMB, Replicas: svc.Replicas,
		ActionsKeySet: rt.sharedEnvHasKey(ctx, *svc, nextActionsKeyEnv),
		SharedCache:   hasRedisLink(svc),
	}
	if _, ok := svc.Env[nextActionsKeyEnv]; ok {
		f.ActionsKeySet = true
	}
	if svc.Resources != nil {
		f.MemoryMB = int(svc.Resources.MemoryBytes >> 20)
	}
	return f, true, nil
}

func hasRedisLink(svc *store.DesiredService) bool {
	for _, d := range svc.DependsOn {
		l := strings.ToLower(d)
		if strings.Contains(l, "redis") || strings.Contains(l, "valkey") {
			return true
		}
	}
	for _, v := range svc.Env {
		if strings.HasPrefix(v, "redis://") || strings.HasPrefix(v, "rediss://") {
			return true
		}
	}
	return false
}
