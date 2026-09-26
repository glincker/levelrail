package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxPlatformImportBody = 64 << 10

// platformImportRequest carries the source credential in the body only.
// It is never logged, audited or echoed back.
type platformImportRequest struct {
	Platform      string   `json:"platform"`
	URL           string   `json:"url"`
	Token         string   `json:"token"`
	InsecureTLS   bool     `json:"insecure_tls,omitempty"`
	AllowPrivate  bool     `json:"allow_private,omitempty"`
	AllowLoopback bool     `json:"allow_loopback,omitempty"`
	Only          []string `json:"only,omitempty"`
	Collision     string   `json:"collision,omitempty"`
}

func envTrue(name string) bool {
	v, err := strconv.ParseBool(os.Getenv(name))
	return err == nil && v
}

// platformImportPolicy turns the request's private-network opt-ins into a
// policy. Each needs both the operator's env var and the per-request flag.
func platformImportPolicy(req platformImportRequest) (platformimport.NetworkPolicy, error) {
	if req.AllowPrivate && !envTrue(platformimport.AllowPrivateEnv) {
		return platformimport.NetworkPolicy{}, fmt.Errorf("allow_private needs %s=true on the control plane", platformimport.AllowPrivateEnv)
	}
	if req.AllowLoopback && !envTrue(platformimport.AllowLoopbackEnv) {
		return platformimport.NetworkPolicy{}, fmt.Errorf("allow_loopback needs %s=true on the control plane", platformimport.AllowLoopbackEnv)
	}
	return platformimport.NetworkPolicy{AllowPrivate: req.AllowPrivate, AllowLoopback: req.AllowLoopback}, nil
}

func (rt *Router) decodePlatformImport(w http.ResponseWriter, r *http.Request) (platformImportRequest, platformimport.Source, bool) {
	var req platformImportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPlatformImportBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return req, nil, false
	}
	platform, err := platformimport.ParsePlatform(req.Platform)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return req, nil, false
	}
	if req.Collision != "" && req.Collision != platformimport.CollisionSuffix && req.Collision != platformimport.CollisionSkip {
		writeError(w, http.StatusBadRequest, "collision must be suffix or skip")
		return req, nil, false
	}
	policy, err := platformImportPolicy(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return req, nil, false
	}
	if u, perr := url.Parse(req.URL); perr == nil && (policy.AllowPrivate || policy.AllowLoopback) {
		rt.logger.Warn("api: platform import: private network opt-in used", slog.String("platform", req.Platform), slog.String("host", u.Hostname()), slog.Bool("allow_private", policy.AllowPrivate), slog.Bool("allow_loopback", policy.AllowLoopback))
	}
	src, err := platformimport.NewSource(platform, req.URL, req.Token, platformimport.ClientOptions{Policy: policy, Insecure: req.InsecureTLS})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return req, nil, false
	}
	return req, src, true
}

func (rt *Router) buildImportPlan(ctx context.Context, req platformImportRequest, src platformimport.Source) (*platformimport.Plan, error) {
	disc, err := src.Discover(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the source platform: %w", err)
	}
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	opts := platformimport.PlanOptions{Only: req.Only, Collision: req.Collision,
		ExistingApps: map[string]string{}, TakenApps: map[string]bool{}, ExistingDatabases: map[string]string{}}
	for _, s := range svcs {
		opts.TakenApps[s.Name] = true
		if id := s.Labels[platformimport.LabelSourceID]; id != "" {
			opts.ExistingApps[id] = s.Name
		}
	}
	for _, d := range dbs {
		opts.ExistingDatabases[d.Name] = d.Engine + ":" + d.Version
	}
	return platformimport.BuildPlan(disc, opts), nil
}

func (rt *Router) writeImportError(w http.ResponseWriter, err error, token string) {
	msg := err.Error()
	if token != "" {
		msg = strings.ReplaceAll(msg, token, "[redacted]")
	}
	rt.logger.Warn("api: platform import failed", slog.String("error", msg))
	writeError(w, http.StatusBadGateway, msg)
}

// handleDiscoverPlatformImport handles POST /api/v1/imports/platform/discover.
func (rt *Router) handleDiscoverPlatformImport(w http.ResponseWriter, r *http.Request) {
	req, src, ok := rt.decodePlatformImport(w, r)
	if !ok {
		return
	}
	plan, err := rt.buildImportPlan(r.Context(), req, src)
	if err != nil {
		rt.writeImportError(w, err, req.Token)
		return
	}
	stripBindMounts(rt, r, plan)
	writeJSON(w, http.StatusOK, plan.Report)
}

// handleApplyPlatformImport handles POST /api/v1/imports/platform/apply.
func (rt *Router) handleApplyPlatformImport(w http.ResponseWriter, r *http.Request) {
	req, src, ok := rt.decodePlatformImport(w, r)
	if !ok {
		return
	}
	plan, err := rt.buildImportPlan(r.Context(), req, src)
	if err != nil {
		rt.writeImportError(w, err, req.Token)
		return
	}
	stripBindMounts(rt, r, plan)
	needsSecrets := false
	for _, a := range plan.Apps {
		if len(a.Secrets) > 0 {
			needsSecrets = true
		}
	}
	if needsSecrets && rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
		return
	}
	report := platformimport.Apply(r.Context(), plan, &importApplier{rt: rt})
	rt.nudgeReconciler()
	rt.logger.Info("api: platform import applied", slog.String("platform", req.Platform), slog.Any("counts", report.Counts))
	writeJSON(w, http.StatusOK, report)
}

// stripBindMounts drops host bind mounts for callers without the root
// ability, since persisting one is a host-filesystem capability.
func stripBindMounts(rt *Router, r *http.Request, plan *platformimport.Plan) {
	if rt.callerHasAbility(r, AbilityRoot) {
		return
	}
	for i := range plan.Apps {
		kept := plan.Apps[i].Volumes[:0]
		dropped := 0
		for _, v := range plan.Apps[i].Volumes {
			if v.HostPath != "" {
				dropped++
				continue
			}
			kept = append(kept, v)
		}
		plan.Apps[i].Volumes = kept
		if dropped == 0 {
			continue
		}
		for j := range plan.Report.Items {
			it := &plan.Report.Items[j]
			if it.Kind == "app" && it.SourceID == plan.Apps[i].SourceID {
				it.Status = platformimport.StatusNeedsAttention
				it.Reasons = append(it.Reasons, fmt.Sprintf("%d bind mount(s) were skipped: they need the root ability", dropped))
				it.Manual = append(it.Manual, "re-run with a root token or add the bind mounts by hand")
			}
		}
	}
}

type importApplier struct{ rt *Router }

func (a *importApplier) projectID(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	list, err := a.rt.projects.ListProjects(ctx)
	if err != nil {
		return "", fmt.Errorf("list projects: %w", err)
	}
	for _, p := range list {
		if strings.EqualFold(p.Name, name) {
			return p.ID, nil
		}
	}
	id, err := randomProjectID()
	if err != nil {
		return "", fmt.Errorf("generate project id: %w", err)
	}
	p := store.Project{ID: id, Name: name, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := a.rt.projects.SaveProject(ctx, p); err != nil {
		return "", fmt.Errorf("create project: %w", err)
	}
	return id, nil
}

func planToAppResource(p platformimport.AppPlan) appResource {
	res := appResource{Name: p.Name, Image: p.Image, Port: p.Port, Domains: p.Domains, Env: p.Env, Replicas: p.Replicas, Labels: p.Labels}
	if p.MemoryBytes > 0 || p.NanoCPUs > 0 {
		res.Resources = &store.ServiceResources{MemoryBytes: p.MemoryBytes, NanoCPUs: p.NanoCPUs}
	}
	if h := p.Health; h != nil {
		probe := &store.ServiceProbe{Path: h.Path, Interval: time.Duration(h.IntervalSeconds) * time.Second, Timeout: time.Duration(h.TimeoutSeconds) * time.Second, Failures: h.Failures}
		res.Health = &store.ServiceHealth{Readiness: probe}
	}
	return res
}

func (a *importApplier) CreateApp(ctx context.Context, p platformimport.AppPlan) ([]string, error) {
	rt := a.rt
	res := planToAppResource(p)
	if err := validateAppResource(res); err != nil {
		return nil, fmt.Errorf("invalid app: %w", err)
	}
	projectID, err := a.projectID(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	if _, err := rt.apps.GetDesiredService(ctx, p.Name); err == nil {
		return nil, errors.New("an app with this name already exists")
	} else if !errors.Is(err, store.ErrServiceNotFound) {
		return nil, fmt.Errorf("check existing app: %w", err)
	}
	secretKeys := make([]string, 0, len(p.Secrets))
	for k, v := range p.Secrets {
		if err := rt.secrets.SetValueGuarded(ctx, p.Name, k, v, false); err != nil {
			return nil, fmt.Errorf("store secret %q: %w", k, err)
		}
		secretKeys = append(secretKeys, k)
	}
	sort.Strings(secretKeys)

	desired := res.toDesiredService()
	desired.SecretEnv = store.SecretEnvRefsFromNames(secretKeys)
	for _, v := range p.Volumes {
		if v.HostPath != "" {
			desired.BindMounts = append(desired.BindMounts, store.ServiceBindMount{HostPath: v.HostPath, ContainerPath: v.ContainerPath, ReadOnly: v.ReadOnly})
			continue
		}
		desired.Volumes = append(desired.Volumes, store.ServiceVolume{Name: "app-" + p.Name + "-" + v.Name, ContainerPath: v.ContainerPath})
	}
	var warnings []string
	err = rt.apps.SaveDesiredService(ctx, desired)
	var taken *store.ErrDomainTaken
	if errors.As(err, &taken) {
		warnings = append(warnings, "domains were not attached: "+taken.Error())
		desired.Domains = nil
		err = rt.apps.SaveDesiredService(ctx, desired)
	}
	if err != nil {
		return warnings, fmt.Errorf("save app: %w", err)
	}
	if projectID != "" {
		if err := rt.apps.UpdateServiceProject(ctx, p.Name, projectID); err != nil {
			return warnings, fmt.Errorf("assign project: %w", err)
		}
	}
	if _, err := rt.ensureAppLinked(ctx, p.Name, p.Name); err != nil {
		return warnings, fmt.Errorf("link app: %w", err)
	}
	return warnings, nil
}

func (a *importApplier) CreateDatabase(ctx context.Context, p platformimport.DatabasePlan) ([]string, error) {
	rt := a.rt
	res := databaseResource{Name: p.Name, Engine: p.Engine, Version: p.Version}
	if err := validateDatabaseResource(res); err != nil {
		return nil, fmt.Errorf("invalid database: %w", err)
	}
	if _, err := rt.databases.GetDesiredDatabase(ctx, p.Name); err == nil {
		return nil, errors.New("a database with this name already exists")
	} else if !errors.Is(err, store.ErrDatabaseNotFound) {
		return nil, fmt.Errorf("check existing database: %w", err)
	}
	projectID, err := a.projectID(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	if err := rt.databases.SaveDesiredDatabase(ctx, res.toDesiredDatabase()); err != nil {
		return nil, fmt.Errorf("save database: %w", err)
	}
	if projectID != "" {
		if err := rt.databases.UpdateDatabaseProject(ctx, p.Name, projectID); err != nil {
			return nil, fmt.Errorf("assign project: %w", err)
		}
	}
	return nil, nil
}
