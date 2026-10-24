package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/migrateverify"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	maxMigrationVerifyBody   = 64 << 10
	envVerifyCredentialTTL   = "APP_MIGRATE_VERIFY_CREDENTIAL_TTL" //nolint:gosec // env var name, not a credential
	defaultVerifyCredentials = 30 * time.Minute
)

// MigrationVerifyStore keeps the latest parity report.
type MigrationVerifyStore interface {
	SaveMigrationVerifyReport(ctx context.Context, r store.MigrationVerifyReport) error
	GetMigrationVerifyReport(ctx context.Context) (store.MigrationVerifyReport, error)
}

// parityVerifyRequest is the source credential plus the probe settings.
// Recheck reuses the last request, whose credential is held in memory only.
type parityVerifyRequest struct {
	platformImportRequest
	Recheck             bool   `json:"recheck,omitempty"`
	SkipProbes          bool   `json:"skip_probes,omitempty"`
	ProbeTimeoutSeconds int    `json:"probe_timeout_seconds,omitempty"`
	ProbeConcurrency    int    `json:"probe_concurrency,omitempty"`
	AllowPrivateProbes  bool   `json:"allow_private_probes,omitempty"`
	TargetIP            string `json:"target_ip,omitempty"`
	SourceIP            string `json:"source_ip,omitempty"`
}

type parityVerifyResponse struct {
	migrateverify.Report
	// CanRecheck is true while the source credential from the last run is still held in memory.
	CanRecheck bool `json:"can_recheck"`
}

// verifyLive holds the last verify request in memory so Re-check needs no new credential.
type verifyLive struct {
	mu      sync.Mutex
	req     *parityVerifyRequest
	expires time.Time
	running bool
}

func (v *verifyLive) remember(req parityVerifyRequest) {
	v.mu.Lock()
	defer v.mu.Unlock()
	switch req.Platform {
	case string(platformimport.Coolify), string(platformimport.Dokploy), string(platformimport.CapRover), string(platformimport.Heroku):
		cp := req
		cp.Recheck = false
		v.req, v.expires = &cp, time.Now().Add(envDuration(envVerifyCredentialTTL, defaultVerifyCredentials))
	default:
		v.req = nil
	}
}

func (v *verifyLive) recall() (parityVerifyRequest, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.req == nil || time.Now().After(v.expires) {
		v.req = nil
		return parityVerifyRequest{}, false
	}
	return *v.req, true
}

func (v *verifyLive) canRecheck() bool {
	_, ok := v.recall()
	return ok
}

func (v *verifyLive) begin() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.running {
		return false
	}
	v.running = true
	return true
}

func (v *verifyLive) end() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.running = false
}

func (rt *Router) migrationVerifyProbeOptions(req parityVerifyRequest) (migrateverify.ProbeOptions, error) {
	opts := migrateverify.ProbeOptionsFromEnv(func(k string) (string, bool) { return os.LookupEnv(k) })
	if req.ProbeTimeoutSeconds > 0 {
		opts.Timeout = time.Duration(req.ProbeTimeoutSeconds) * time.Second
	}
	if req.ProbeConcurrency > 0 {
		opts.Concurrency = req.ProbeConcurrency
	}
	if req.AllowPrivateProbes && !envTrue(migrateverify.EnvAllowPrivateProbe) {
		return opts, fmt.Errorf("allow_private_probes needs %s=true on the control plane", migrateverify.EnvAllowPrivateProbe)
	}
	for _, addr := range []string{req.TargetIP, req.SourceIP} {
		if err := migrateverify.ValidateAddr(addr); err != nil {
			return opts, err
		}
	}
	opts.AllowPrivate, opts.TargetAddr, opts.SourceAddr = req.AllowPrivateProbes, req.TargetIP, req.SourceIP
	return migrateverify.ClampProbeOptions(opts), nil
}

// migrationVerifyTarget reads this instance as names only.
func (rt *Router) migrationVerifyTarget(ctx context.Context) (migrateverify.Target, error) {
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return migrateverify.Target{}, fmt.Errorf("list apps: %w", err)
	}
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return migrateverify.Target{}, fmt.Errorf("list databases: %w", err)
	}
	var t migrateverify.Target
	for i := range svcs {
		s := &svcs[i]
		app := migrateverify.TargetApp{Name: s.Name, SourceKey: s.Labels[platformimport.LabelSourceID],
			Staged: s.Labels[platformimport.LabelSession] != "" && s.Suspended, Domains: s.Domains}
		for k := range existingServiceEnvKeys(s) {
			app.EnvKeys = append(app.EnvKeys, k)
		}
		sort.Strings(app.EnvKeys)
		for _, v := range s.Volumes {
			app.Volumes = append(app.Volumes, v.ContainerPath)
		}
		for _, b := range s.BindMounts {
			app.Volumes = append(app.Volumes, b.ContainerPath)
		}
		tasks, err := rt.scheduledTasks.ListScheduledTasksForService(ctx, s.Name)
		if err != nil {
			return migrateverify.Target{}, fmt.Errorf("list jobs of %q: %w", s.Name, err)
		}
		for _, task := range tasks {
			app.Jobs = append(app.Jobs, migrateverify.Job{Name: task.Name, Schedule: task.Schedule, Enabled: task.Enabled})
		}
		t.Apps = append(t.Apps, app)
	}
	for _, d := range dbs {
		t.DBs = append(t.DBs, migrateverify.DB{Name: d.Name, Engine: d.Engine, Version: d.Version})
	}
	return t, nil
}

func filterVerifySource(s migrateverify.Source, only []string) migrateverify.Source {
	if len(only) == 0 {
		return s
	}
	keep := map[string]bool{}
	for _, o := range only {
		keep[strings.ToLower(strings.TrimSpace(o))] = true
	}
	var apps []migrateverify.SourceApp
	for _, a := range s.Apps {
		if keep[strings.ToLower(a.ID)] || keep[strings.ToLower(a.Name)] {
			apps = append(apps, a)
		}
	}
	s.Apps = apps
	return s
}

func (rt *Router) respondVerify(w http.ResponseWriter, status int, rep migrateverify.Report) {
	writeJSON(w, status, parityVerifyResponse{Report: rep, CanRecheck: rt.verifyLive.canRecheck()})
}

// handleMigrationVerify handles POST /api/v1/migrations/verify. It reads the
// source and this instance, probes each domain on both sides, stores the
// report and returns it. Nothing is created or changed.
func (rt *Router) handleMigrationVerify(w http.ResponseWriter, r *http.Request) {
	var req parityVerifyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxMigrationVerifyBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	defer req.wipeSecrets()
	if req.Recheck {
		saved, ok := rt.verifyLive.recall()
		if !ok {
			writeError(w, http.StatusConflict, "the source credential from the last check has expired: send it again")
			return
		}
		req = saved
	} else if err := req.normalizeSSH(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	opts, err := rt.migrationVerifyProbeOptions(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	src, ok := rt.openImportSource(w, req.platformImportRequest)
	if !ok {
		return
	}
	if !rt.verifyLive.begin() {
		writeError(w, http.StatusConflict, "a verification is already running")
		return
	}
	defer rt.verifyLive.end()

	disc, err := src.Discover(r.Context())
	if err != nil {
		rt.writeImportError(w, fmt.Errorf("reading the source platform: %w", err), req.Token)
		return
	}
	target, err := rt.migrationVerifyTarget(r.Context())
	if err != nil {
		rt.internalError(w, "api: migration verify: read target failed", err)
		return
	}
	var prober *migrateverify.Prober
	if !req.SkipProbes {
		prober = &migrateverify.Prober{Opts: opts}
	}
	rep := migrateverify.Build(r.Context(), filterVerifySource(migrateverify.SnapshotSource(disc), req.Only), target, prober,
		migrateverify.ThresholdsFromEnv(func(k string) (string, bool) { return os.LookupEnv(k) }), time.Now())
	body, err := json.Marshal(rep)
	if err != nil {
		rt.internalError(w, "api: migration verify: encode report failed", err)
		return
	}
	if err := rt.migrationVerify.SaveMigrationVerifyReport(r.Context(), store.MigrationVerifyReport{Platform: rep.Platform, GeneratedAt: rep.GeneratedAt, Verdict: rep.Verdict, Body: body}); err != nil {
		rt.internalError(w, "api: migration verify: store report failed", err)
		return
	}
	rt.verifyLive.remember(req)
	rt.logger.Info("api: migration verify finished", slog.String("platform", rep.Platform), slog.String("verdict", rep.Verdict),
		slog.Int("blocking", rep.Counts.Blocking), slog.Int("warning", rep.Counts.Warning), slog.Int("probes", len(rep.Probes)))
	rt.respondVerify(w, http.StatusOK, rep)
}

// handleLatestMigrationVerify handles GET /api/v1/migrations/verify/latest.
func (rt *Router) handleLatestMigrationVerify(w http.ResponseWriter, r *http.Request) {
	stored, err := rt.migrationVerify.GetMigrationVerifyReport(r.Context())
	if errors.Is(err, store.ErrMigrationVerifyNotFound) {
		writeError(w, http.StatusNotFound, "no migration verification has been run yet")
		return
	}
	if err != nil {
		rt.internalError(w, "api: migration verify: read report failed", err)
		return
	}
	var rep migrateverify.Report
	if err := json.Unmarshal(stored.Body, &rep); err != nil {
		rt.internalError(w, "api: migration verify: decode stored report failed", err)
		return
	}
	rt.respondVerify(w, http.StatusOK, rep)
}
