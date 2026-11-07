package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Stable finding ids for GET /api/v1/apps/{name}/next-advisor.
const (
	nextFindReplicasNoCache = "next-cache-missing-replicas"
	nextFindCacheNoDB       = "next-cache-no-database"
	nextFindCacheNoCA       = "next-cache-no-ca"
	nextFindSkewUserID      = "skew-user-deployment-id"
	nextFindBuildMemory     = "build-memory-near-host"
	nextFindVitalsOff       = "web-vitals-off"
	nextFindGraceOff        = "asset-grace-off"

	envNextAdvisorBuildMemPct = "APP_NEXT_ADVISOR_BUILD_MEM_PCT"
	defaultBuildMemPct        = 90

	nextCacheURLEnv = "LEVELRAIL_NEXT_CACHE_URL"
	nextCacheCAEnv  = "LEVELRAIL_NEXT_CACHE_CA"
	nextRedisEnv    = "REDIS_URL"
	nextDeployIDEnv = "NEXT_DEPLOYMENT_ID"
)

type nextAdvisorFinding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Fix      string `json:"fix"`
}

type nextAdvisorResponse struct {
	Detected bool                 `json:"detected"`
	Findings []nextAdvisorFinding `json:"findings"`
}

// handleNextAdvisor handles GET /api/v1/apps/{name}/next-advisor: checks of the saved
// app and its latest successful release, no build needed. Findings carry names, never values.
func (rt *Router) handleNextAdvisor(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()
	svc, err := rt.apps.GetDesiredService(ctx, name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: next advisor: load app failed", err, slog.String("name", name))
		return
	}
	var latest *store.DeployAttempt
	detected := svc.NextCache != ""
	if rt.deployAttempts != nil {
		attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
		if err != nil {
			rt.internalError(w, "api: next advisor: list attempts failed", err, slog.String("name", name))
			return
		}
		for i, a := range attempts {
			if latest == nil && a.Status == store.DeployAttemptStatusSucceeded {
				latest = &attempts[i]
			}
			if !detected && a.DetectedFramework != "" {
				detected = strings.EqualFold(a.DetectedFramework, "Next.js")
				break
			}
		}
	}
	resp := nextAdvisorResponse{Detected: detected, Findings: []nextAdvisorFinding{}}
	if detected {
		resp.Findings = rt.nextAdvisorFindings(ctx, svc, latest)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (rt *Router) nextAdvisorFindings(ctx context.Context, svc *store.DesiredService, latest *store.DeployAttempt) []nextAdvisorFinding {
	out := []nextAdvisorFinding{}
	out = append(out, nextCacheFindings(ctx, svc, rt.apps)...)
	if f, ok := nextSkewFinding(svc, latest); ok {
		out = append(out, f)
	}
	if f, ok := rt.nextBuildMemoryFinding(ctx); ok {
		out = append(out, f)
	}
	if f, ok := rt.nextVitalsFinding(ctx, svc.Name); ok {
		out = append(out, f)
	}
	if rt.runtimeSettings != nil && rt.runtimeSettings.Int(ctx, deploy.KeyNextAssetGraceSecs) <= 0 {
		out = append(out, nextAdvisorFinding{
			ID: nextFindGraceOff, Severity: "warn",
			Title:  "Old release is not kept after a deploy",
			Detail: "The previous release hold window is 0, so browser tabs on the old build lose their static chunks and data requests the moment the new release goes live.",
			Fix:    "Set the runtime setting " + deploy.KeyNextAssetGraceSecs + " above 0 (the default is 1800 seconds).",
		})
	}
	return out
}

func nextCacheFindings(ctx context.Context, svc *store.DesiredService, apps any) []nextAdvisorFinding {
	var out []nextAdvisorFinding
	_, urlLinked := svc.DatabaseEnv[nextCacheURLEnv]
	_, redisLinked := svc.DatabaseEnv[nextRedisEnv]
	_, urlEnv := svc.Env[nextCacheURLEnv]
	_, redisEnv := svc.Env[nextRedisEnv]
	usesRedis := svc.NextCache == spec.NextCacheRedis || urlLinked || urlEnv
	if svc.Replicas > 1 && !usesRedis {
		out = append(out, nextAdvisorFinding{
			ID: nextFindReplicasNoCache, Severity: "error",
			Title:  "Replicas do not share a Next.js cache",
			Detail: "This app runs more than one replica without the shared Redis cache, so ISR pages and the data cache diverge between replicas and users can see different content.",
			Fix:    "levelrail apps next-cache set " + svc.Name + " redis",
		})
	}
	if svc.NextCache != spec.NextCacheRedis {
		return out
	}
	if !urlLinked && !redisLinked && !urlEnv && !redisEnv {
		out = append(out, nextAdvisorFinding{
			ID: nextFindCacheNoDB, Severity: "warn",
			Title:  "Shared cache has no Redis database",
			Detail: "next_cache is redis but the app has no linked database or connection env var for it, so the cache cannot reach Redis.",
			Fix:    "levelrail databases link <redis-database> " + svc.Name,
		})
		return out
	}
	dbs, ok := apps.(deploy.NextCacheDatabases)
	if !ok || !urlLinked {
		return out
	}
	// Ask the deploy helper what it would link, so the rule never drifts from it.
	probe := *svc
	probe.DatabaseEnv = make(map[string]store.DatabaseEnvRef, len(svc.DatabaseEnv))
	for k, v := range svc.DatabaseEnv {
		probe.DatabaseEnv[k] = v
	}
	deploy.EnsureNextCacheCALink(ctx, &probe, dbs)
	_, had := svc.DatabaseEnv[nextCacheCAEnv]
	_, would := probe.DatabaseEnv[nextCacheCAEnv]
	if would && !had {
		out = append(out, nextAdvisorFinding{
			ID: nextFindCacheNoCA, Severity: "warn",
			Title:  "Redis uses TLS but the app has no CA link",
			Detail: "The linked Redis database has TLS on, but " + nextCacheCAEnv + " is not linked to its tls_ca field, so the cache connection cannot verify the server.",
			Fix:    "Redeploy the app to add the CA link, or link the database tls_ca field to " + nextCacheCAEnv + ".",
		})
	}
	return out
}

// nextSkewFinding fires when NEXT_DEPLOYMENT_ID holds something other than the id the
// platform injects (the deploy attempt id or its commit), which is a user-set value.
func nextSkewFinding(svc *store.DesiredService, latest *store.DeployAttempt) (nextAdvisorFinding, bool) {
	v, set := svc.Env[nextDeployIDEnv]
	if !set || v == "" {
		return nextAdvisorFinding{}, false
	}
	if latest != nil && (v == latest.ID || (latest.CommitSHA != "" && v == latest.CommitSHA)) {
		return nextAdvisorFinding{}, false
	}
	return nextAdvisorFinding{
		ID: nextFindSkewUserID, Severity: "warn",
		Title:  "Skew protection is off: " + nextDeployIDEnv + " is set by you",
		Detail: nextDeployIDEnv + " is set in the app env, so the platform does not inject a per-deploy id and an old browser tab cannot be matched to its build.",
		Fix:    "Remove " + nextDeployIDEnv + " from the app env so each deploy gets its own id.",
	}, true
}

func (rt *Router) nextBuildMemoryFinding(ctx context.Context) (nextAdvisorFinding, bool) {
	if rt.runtimeSettings == nil {
		return nextAdvisorFinding{}, false
	}
	capMB := rt.runtimeSettings.Int(ctx, runtimesettings.KeyBuildsMemoryLimitMB)
	if capMB <= 0 {
		return nextAdvisorFinding{}, false
	}
	read := rt.hostMemory
	if read == nil {
		read = rt.doctorHostMemory
	}
	if read == nil {
		read = telemetry.HostMemoryBytes
	}
	total, _, err := read()
	if err != nil || total <= 0 {
		return nextAdvisorFinding{}, false
	}
	hostMB := int(total >> 20)
	pct := envInt(envNextAdvisorBuildMemPct, defaultBuildMemPct)
	if capMB*100 < hostMB*pct {
		return nextAdvisorFinding{}, false
	}
	return nextAdvisorFinding{
		ID: nextFindBuildMemory, Severity: "warn",
		Title:  "Build memory cap is close to host memory",
		Detail: "builds.memory_limit_mb is " + strconv.Itoa(capMB) + " MiB on a host with " + strconv.Itoa(hostMB) + " MiB, so a Next.js build can starve the running apps.",
		Fix:    "Set builds.memory_limit_mb to 0 (auto) or well under the host memory, or build in CI and deploy the image.",
	}, true
}

func (rt *Router) nextVitalsFinding(ctx context.Context, app string) (nextAdvisorFinding, bool) {
	db, ok := rt.vitalsDB()
	if !ok {
		return nextAdvisorFinding{}, false
	}
	cfg, found, err := db.GetVitalsConfig(ctx, app)
	if err != nil || (found && cfg.Enabled) {
		return nextAdvisorFinding{}, false
	}
	return nextAdvisorFinding{
		ID: nextFindVitalsOff, Severity: "info",
		Title:  "Web vitals are not collected",
		Detail: "Web vitals collection is off for this app, so there is no real-user LCP, INP or CLS data per route.",
		Fix:    "Turn on the Web vitals card for this app and add its beacon snippet.",
	}, true
}
