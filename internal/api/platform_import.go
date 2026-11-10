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
	"github.com/GLINCKER/levelrail/internal/platformimport/dokployextras"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxPlatformImportBody = 64 << 10

// platformImportRequest carries the source credential in the body only.
// It is never logged, audited or echoed back.
type platformImportRequest struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
	Token    string `json:"token"`
	// Snapshot is pasted source content (docker inspect JSON, a compose file or a
	// dokku env dump) used in place of a URL and token. It holds env, so treat it like Token.
	Snapshot string `json:"snapshot,omitempty"`
	// AppName names the one app a dokku env dump describes.
	AppName       string   `json:"dokku_app_name,omitempty"`
	InsecureTLS   bool     `json:"insecure_tls,omitempty"`
	AllowPrivate  bool     `json:"allow_private,omitempty"`
	AllowLoopback bool     `json:"allow_loopback,omitempty"`
	Only          []string `json:"only,omitempty"`
	Collision     string   `json:"collision,omitempty"`
	// DBVersions picks the engine version for source databases that report none.
	DBVersions map[string]string `json:"db_versions,omitempty"`
	// TeamID scopes a Vercel import to one team (team_... or slug).
	TeamID string `json:"team_id,omitempty"`
	// SSH is the login of a docker-ssh source: read-only access to its Docker socket.
	SSH *sshConnection `json:"ssh,omitempty"`
	// IncludeCredentials lets apply read the source's backup keys and notify URLs and pass
	// them straight into the create calls. Off by default; the route needs write:sensitive.
	IncludeCredentials bool `json:"include_credentials,omitempty"`
	// ExtrasOnly skips the app and database import and applies only backups and channels.
	ExtrasOnly bool `json:"extras_only,omitempty"`
	// ImportExtras applies the planned backups and channels alongside the apps.
	ImportExtras bool `json:"import_extras,omitempty"`
	// EnableBackupSchedules keeps imported schedules running; they are created paused otherwise.
	EnableBackupSchedules bool `json:"enable_backup_schedules,omitempty"`
	// EnableImportedChannels creates imported alert channels enabled; they are created disabled otherwise.
	EnableImportedChannels bool `json:"enable_imported_channels,omitempty"`
	// Journal is an earlier run's records, so a re-run resumes instead of creating twice.
	Journal []dokployextras.Record `json:"journal,omitempty"`
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
	if err := req.normalizeSSH(); err != nil {
		req.wipeSecrets()
		writeError(w, http.StatusBadRequest, err.Error())
		return req, nil, false
	}
	src, ok := rt.openImportSource(w, req)
	return req, src, ok
}

// openFileImportSource builds a source from pasted content (docker inspect
// output, a compose file or a dokku env dump), which never touches the network.
func openFileImportSource(w http.ResponseWriter, platform platformimport.Platform, req platformImportRequest) (platformimport.Source, bool) {
	if strings.TrimSpace(req.Snapshot) == "" {
		writeError(w, http.StatusBadRequest, "snapshot is required for the "+string(platform)+" platform")
		return nil, false
	}
	switch platform {
	case platformimport.Compose:
		return platformimport.ComposeSource{Data: []byte(req.Snapshot)}, true
	case platformimport.Dokku:
		if strings.TrimSpace(req.AppName) == "" {
			writeError(w, http.StatusBadRequest, "dokku_app_name is required for the dokku platform")
			return nil, false
		}
		return platformimport.DokkuEnvSource{AppName: strings.TrimSpace(req.AppName), Data: req.Snapshot}, true
	default:
		return platformimport.DockerSource{Data: []byte(req.Snapshot)}, true
	}
}

// openImportSource validates a decoded request and builds the read-only
// source client for it. It writes the error response itself.
func (rt *Router) openImportSource(w http.ResponseWriter, req platformImportRequest) (platformimport.Source, bool) {
	platform, err := platformimport.ParsePlatform(req.Platform)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	if req.Collision != "" && req.Collision != platformimport.CollisionSuffix && req.Collision != platformimport.CollisionSkip {
		writeError(w, http.StatusBadRequest, "collision must be suffix or skip")
		return nil, false
	}
	if req.TeamID != "" && platform != platformimport.Vercel {
		writeError(w, http.StatusBadRequest, "team_id is only supported for the vercel platform")
		return nil, false
	}
	if platform == platformimport.DockerSSH {
		return rt.openDockerSSHSource(w, req)
	}
	if platform == platformimport.Docker || platform == platformimport.Compose || platform == platformimport.Dokku {
		return openFileImportSource(w, platform, req)
	}
	policy, err := platformImportPolicy(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	if u, perr := url.Parse(req.URL); perr == nil && (policy.AllowPrivate || policy.AllowLoopback) {
		rt.logger.Warn("api: platform import: private network opt-in used", slog.String("platform", req.Platform), slog.String("host", u.Hostname()), slog.Bool("allow_private", policy.AllowPrivate), slog.Bool("allow_loopback", policy.AllowLoopback))
	}
	src, err := platformimport.NewSource(platform, req.URL, req.Token, platformimport.ClientOptions{Policy: policy, Insecure: req.InsecureTLS, Team: req.TeamID})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	return src, true
}

func (rt *Router) buildImportPlan(ctx context.Context, req platformImportRequest, src platformimport.Source) (*platformimport.Plan, *platformimport.Discovery, error) {
	disc, err := src.Discover(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the source platform: %w", err)
	}
	opts, _, err := rt.importPlanOptions(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	return platformimport.BuildPlan(disc, opts), disc, nil
}

// importPlanOptions reads what already exists here, so a plan can mark
// repeats and name collisions. The second result maps each routed domain
// (lowercase) to the app that holds it.
func (rt *Router) importPlanOptions(ctx context.Context, req platformImportRequest) (platformimport.PlanOptions, map[string]string, error) {
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return platformimport.PlanOptions{}, nil, fmt.Errorf("list apps: %w", err)
	}
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return platformimport.PlanOptions{}, nil, fmt.Errorf("list databases: %w", err)
	}
	opts := platformimport.PlanOptions{Only: req.Only, Collision: req.Collision,
		ExistingApps: map[string]string{}, TakenApps: map[string]bool{}, ExistingDatabases: map[string]string{}}
	domains := map[string]string{}
	for _, s := range svcs {
		opts.TakenApps[s.Name] = true
		if id := s.Labels[platformimport.LabelSourceID]; id != "" {
			opts.ExistingApps[id] = s.Name
		}
		for _, d := range s.Domains {
			domains[strings.ToLower(d)] = s.Name
		}
	}
	for _, d := range dbs {
		opts.ExistingDatabases[d.Name] = d.Engine + ":" + d.Version
	}
	if opts.ExistingEnvironments, err = rt.existingImportEnvironments(ctx); err != nil {
		return platformimport.PlanOptions{}, nil, err
	}
	if opts.DBVersions, opts.LatestDBVersions, err = rt.importDBVersions(req.DBVersions); err != nil {
		return platformimport.PlanOptions{}, nil, err
	}
	return opts, domains, nil
}

// existingImportEnvironments keys every environment as "project/environment"
// so a re-run skips the ones an earlier run created.
func (rt *Router) existingImportEnvironments(ctx context.Context) (map[string]bool, error) {
	out := map[string]bool{}
	projects, err := rt.projects.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	for _, p := range projects {
		envs, err := rt.environments.ListEnvironmentsByProject(ctx, p.ID)
		if err != nil {
			return nil, fmt.Errorf("list environments: %w", err)
		}
		for _, e := range envs {
			out[platformimport.EnvironmentKey(p.Name, e.Name)] = true
		}
	}
	return out, nil
}

func (rt *Router) writeImportError(w http.ResponseWriter, err error, token string) {
	msg := err.Error()
	if token != "" {
		msg = strings.ReplaceAll(msg, token, "[redacted]")
	}
	rt.logger.Warn("api: platform import failed", slog.String("error", msg))
	var upstream *platformimport.APIError
	if errors.As(err, &upstream) && (upstream.Status == http.StatusUnauthorized || upstream.Status == http.StatusForbidden) {
		// A rejected token is the caller's input, not a fault of this server.
		writeError(w, http.StatusBadRequest, fmt.Sprintf("the source rejected the token (HTTP %d): check that it is valid and can read the account, then try again", upstream.Status))
		return
	}
	writeError(w, http.StatusBadGateway, msg)
}

// handleDiscoverPlatformImport handles POST /api/v1/imports/platform/discover.
func (rt *Router) handleDiscoverPlatformImport(w http.ResponseWriter, r *http.Request) {
	req, src, ok := rt.decodePlatformImport(w, r)
	if !ok {
		return
	}
	defer req.wipeSecrets()
	plan, disc, err := rt.buildImportPlan(r.Context(), req, src)
	if err != nil {
		rt.writeImportError(w, err, req.Token)
		return
	}
	stripBindMounts(rt, r, plan)
	resp := platformImportResponse{Report: plan.Report}
	resp.Backups, resp.Channels = importExtras(r.Context(), req, disc)
	writeJSON(w, http.StatusOK, resp)
}

// handleApplyPlatformImport handles POST /api/v1/imports/platform/apply.
func (ip *importInproc) handleApplyPlatformImport(w http.ResponseWriter, r *http.Request) {
	rt := ip.rt
	req, src, ok := rt.decodePlatformImport(w, r)
	if !ok {
		return
	}
	defer req.wipeSecrets()
	plan, disc, err := rt.buildImportPlan(r.Context(), req, src)
	if err != nil {
		rt.writeImportError(w, err, req.Token)
		return
	}
	stripBindMounts(rt, r, plan)
	needsSecrets := false
	if req.ExtrasOnly {
		plan.Apps = nil
	}
	for _, a := range plan.Apps {
		if len(a.Secrets) > 0 {
			needsSecrets = true
		}
	}
	if needsSecrets && rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
		return
	}
	// An empty only list means every app, so an extras-only run skips the app import outright.
	report := plan.Report
	if !req.ExtrasOnly {
		report = platformimport.Apply(r.Context(), plan, &importApplier{rt: rt, providerRefs: rt.callerHasAbility(r, AbilityRoot)})
	}
	resp := platformImportResponse{Report: report}
	resp.Backups, resp.Channels = importExtras(r.Context(), req, disc)
	ip.applyImportExtras(r, req, src, &resp)
	rt.nudgeReconciler()
	rt.logger.Info("api: platform import applied", slog.String("platform", req.Platform), slog.Any("counts", report.Counts), slog.Int("extras_created", len(resp.Journal)))
	writeJSON(w, http.StatusOK, resp)
}

// stripBindMounts drops host bind mounts and cron jobs for callers without the
// root ability: a bind mount is a host-filesystem capability and an imported cron
// runs "sh -c" in the app, which the console exec rule reserves for root.
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
		droppedJobs := len(plan.Apps[i].Crons)
		plan.Apps[i].Crons = nil
		if dropped == 0 && droppedJobs == 0 {
			continue
		}
		for j := range plan.Report.Items {
			it := &plan.Report.Items[j]
			if it.Kind != "app" || it.SourceID != plan.Apps[i].SourceID {
				continue
			}
			it.Status = platformimport.StatusNeedsAttention
			if dropped > 0 {
				it.Reasons = append(it.Reasons, fmt.Sprintf("%d bind mount(s) were skipped: they need the root ability", dropped))
				it.Manual = append(it.Manual, "re-run with a root token or add the bind mounts by hand")
			}
			if droppedJobs > 0 {
				it.Reasons = append(it.Reasons, fmt.Sprintf("%d scheduled job(s) were skipped: jobs run a shell command and need the root ability", droppedJobs))
				it.Manual = append(it.Manual, "a root token can re-run the import or add the jobs in the app's Jobs tab")
			}
		}
	}
}

// importApplier creates imported apps. With a sessionID it stages them:
// suspended, domains held back and the source repository connected, so
// nothing runs or routes until the operator verifies and cuts over.
type importApplier struct {
	rt        *Router
	sessionID string
	// connect lists the managed databases each app (by target name) must join
	// the network of, because a rewritten hostname only resolves there.
	connect map[string][]string
	// providerRefs lets imported secrets hold provider references; only a root caller sets it.
	providerRefs bool
}

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

// environmentID returns the id of the named environment in the named project,
// creating it when missing. It is "" when either name is empty.
func (a *importApplier) environmentID(ctx context.Context, project, name string) (string, error) {
	if project == "" || name == "" {
		return "", nil
	}
	projectID, err := a.projectID(ctx, project)
	if err != nil {
		return "", err
	}
	envs, err := a.rt.environments.ListEnvironmentsByProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("list environments: %w", err)
	}
	for _, e := range envs {
		if strings.EqualFold(e.Name, name) {
			return e.ID, nil
		}
	}
	id, err := randomEnvironmentID()
	if err != nil {
		return "", fmt.Errorf("generate environment id: %w", err)
	}
	e := store.Environment{ID: id, ProjectID: projectID, Name: name, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := a.rt.environments.SaveEnvironment(ctx, e); err != nil {
		return "", fmt.Errorf("create environment: %w", err)
	}
	return id, nil
}

// CreateEnvironment makes the project and environment if they are missing,
// so a re-run is a no-op.
func (a *importApplier) CreateEnvironment(ctx context.Context, p platformimport.EnvironmentPlan) ([]string, error) {
	_, err := a.environmentID(ctx, p.Project, p.Name)
	return nil, err
}

func planToAppResource(p platformimport.AppPlan) appResource {
	res := appResource{Name: p.Name, Image: p.Image, Port: p.Port, Domains: p.Domains, Env: p.Env, Replicas: p.Replicas, Labels: p.Labels}
	if p.MemoryBytes > 0 || p.NanoCPUs > 0 {
		res.Resources = &store.ServiceResources{MemoryBytes: p.MemoryBytes, NanoCPUs: p.NanoCPUs}
	}
	if h := p.Health; h != nil {
		probe := &store.ServiceProbe{Path: h.Path, Exec: h.Exec, Interval: time.Duration(h.IntervalSeconds) * time.Second, Timeout: time.Duration(h.TimeoutSeconds) * time.Second, Failures: h.Failures}
		res.Health = &store.ServiceHealth{Readiness: probe}
	}
	return res
}

func (a *importApplier) CreateApp(ctx context.Context, p platformimport.AppPlan) ([]string, error) {
	rt := a.rt
	res := planToAppResource(p)
	if a.sessionID != "" {
		res.Domains = nil
		res.Labels = map[string]string{}
		for k, v := range p.Labels {
			res.Labels[k] = v
		}
		res.Labels[platformimport.LabelSession] = a.sessionID
	}
	if err := validateAppResource(res); err != nil {
		return nil, fmt.Errorf("invalid app: %w", err)
	}
	projectID, err := a.projectID(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	// Held from the existence check to the row save so a same-name delete cannot wipe these secrets in between.
	unlockName := lockAppName(p.Name)
	defer unlockName()
	if _, err := rt.apps.GetDesiredService(ctx, p.Name); err == nil {
		return nil, errors.New("an app with this name already exists")
	} else if !errors.Is(err, store.ErrServiceNotFound) {
		return nil, fmt.Errorf("check existing app: %w", err)
	}
	for k, v := range p.Secrets {
		if !a.providerRefs && rt.resolvesThroughProvider(ctx, v) {
			return nil, fmt.Errorf("secret %q: %s", k, providerRefNeedsRoot)
		}
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
	envID, err := a.environmentID(ctx, p.Project, p.Environment)
	if err != nil {
		return warnings, err
	}
	if envID != "" {
		if err := rt.environments.SetServiceEnvironment(ctx, p.Name, envID); err != nil {
			return warnings, fmt.Errorf("assign environment: %w", err)
		}
	}
	// Linking stays under the lock: a delete's orphan sweep must not drop the App row between lookup and link.
	if _, err := rt.ensureAppLinked(ctx, p.Name, p.Name); err != nil {
		return warnings, fmt.Errorf("link app: %w", err)
	}
	unlockName()
	warnings = append(warnings, a.createImportedJobs(ctx, p.Name, p.Crons)...)
	if a.sessionID != "" {
		staged, err := a.finishStaging(ctx, p)
		warnings = append(warnings, staged...)
		if err != nil {
			return warnings, err
		}
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
	envID, err := a.environmentID(ctx, p.Project, p.Environment)
	if err != nil {
		return nil, err
	}
	if envID != "" {
		if err := a.rt.setDatabaseEnvironment(ctx, p.Name, envID); err != nil {
			return nil, fmt.Errorf("assign environment: %w", err)
		}
	}
	return nil, nil
}
