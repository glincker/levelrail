package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/oidc"
	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const gitSourceTokenKey = "deploy_token"

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

func envDuration(name string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

// startPipelines builds the pipeline engine and scheduler, attaches them to
// the API router and git webhook path, and runs both until ctx is done.
func startPipelines(ctx context.Context, logger *slog.Logger, b *brand.Brand, db *store.DB, secretsManager *secrets.Manager, client *docker.Client,
	registry *agent.Registry, builder *deploy.Pipeline, nudger *reconcile.Engine, dispatcher *alerting.DeployDispatcher, apiRouter *api.Router) {
	acts := &pipelineActions{db: db, builder: builder, nudger: nudger, notifier: dispatcher, secrets: secretsManager, logger: logger, resolver: client}
	cfg := pipeline.Config{
		Store:      db,
		Actions:    acts,
		Source:     acts,
		RunEnv:     apiRouter.PipelineRunEnv,
		Logger:     logger,
		NamePrefix: b.ShortName,
		Runtime: func(nodeID string) (pipeline.Runtime, error) {
			rt, err := resolveNodeTransport(client, registry, nodeID)
			if err != nil {
				return nil, fmt.Errorf("resolve node %q: %w", nodeID, err)
			}
			return rt, nil
		},
		GitImage:        os.Getenv("APP_PIPELINE_GIT_IMAGE"),
		MaxParallelJobs: envInt("APP_PIPELINE_MAX_PARALLEL_JOBS", 0),
		MaxLogLines:     envInt("APP_PIPELINE_MAX_LOG_LINES", 0),
		StepTimeout:     envDuration("APP_PIPELINE_STEP_TIMEOUT", 0),
		JobTimeout:      envDuration("APP_PIPELINE_JOB_TIMEOUT", 0),
		ApprovalTimeout: envDuration("APP_PIPELINE_APPROVAL_TIMEOUT", 0),
		KeepRuns:        envInt("APP_PIPELINE_KEEP_RUNS", 0),
		Reporter:        apiRouter.PipelineStatusReporter(),
		ReportTimeout:   envDuration("APP_GIT_STATUS_TIMEOUT", 0),
	}
	apiRouter.SetForgeDeployments(db)
	oidcRequestAddr := ""
	if secretsManager != nil {
		cfg.Secrets = secretsManager
		oidcRequestAddr = wireOIDC(ctx, &cfg, apiRouter, secretsManager, client, logger)
	}
	engine := pipeline.New(cfg)
	apiRouter.SetPipelines(db, engine)
	apiRouter.SetPipelineEvents(engine)
	apiRouter.SetPipelineSync(pipeline.NewSyncer(pipeline.SyncConfig{
		Store: db, Source: acts, Fetcher: pipeline.GitFetcher{}, BrandName: b.ShortName, Logger: logger,
	}), db)
	if oidcRequestAddr != "" {
		go serveOIDCTokenRequests(ctx, oidcRequestAddr, engine, logger)
	}

	go func() {
		if err := engine.Run(ctx, envDuration("APP_PIPELINE_TICK_INTERVAL", 2*time.Second)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("pipeline engine stopped", slog.String("error", err.Error()))
		}
	}()
	scheduler := pipeline.NewScheduler(engine)
	go func() {
		if err := scheduler.Run(ctx, envDuration("APP_PIPELINE_SCHEDULER_INTERVAL", 30*time.Second)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("pipeline scheduler stopped", slog.String("error", err.Error()))
		}
	}()
}

// pipelineActions adapts the existing build, deploy, and alerting code to the
// pipeline engine's Actions and Source interfaces.
type pipelineActions struct {
	db       *store.DB
	builder  *deploy.Pipeline
	nudger   deploy.ReconcileNudger
	notifier *alerting.DeployDispatcher
	secrets  *secrets.Manager
	logger   *slog.Logger
	resolver docker.ImageResolver
}

func (a *pipelineActions) RepoInfo(ctx context.Context, app string) (string, string, error) {
	gs, err := a.db.GetGitSource(ctx, app)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		return "", "", pipeline.ErrNoRepo
	}
	if err != nil {
		return "", "", fmt.Errorf("load git source: %w", err)
	}
	token := ""
	if a.secrets != nil {
		key := store.GitSourceSecretsKey(app)
		if ok, err := a.secrets.Exists(ctx, key, gitSourceTokenKey); err == nil && ok {
			if token, err = a.secrets.Resolve(ctx, key, gitSourceTokenKey); err != nil {
				return "", "", fmt.Errorf("resolve deploy token: %w", err)
			}
		}
	}
	return gs.RepoURL, token, nil
}

func (a *pipelineActions) Build(ctx context.Context, req pipeline.BuildRequest, log func(string)) (string, error) {
	if a.builder == nil {
		return "", errors.New("the builder is not configured on this control plane")
	}
	url, token, err := a.RepoInfo(ctx, req.App)
	if err != nil {
		return "", fmt.Errorf("build needs a connected repository: %w", err)
	}
	log("cloning " + url)
	dir, cleanup, err := cloneForBuild(ctx, url, token, req.Ref, req.SHA)
	if err != nil {
		return "", err
	}
	defer cleanup()
	tag, err := a.builder.BuildOnly(ctx, deploy.BuildOnlyRequest{
		Type: req.Type, SourceDir: dir, BaseDirectory: req.Context, Dockerfile: req.Dockerfile, Tag: req.Image + ":" + req.Tag,
	}, func(ev build.ProgressEvent) {
		if ev.Log != "" {
			log(strings.TrimRight(ev.Log, "\n"))
		}
	})
	if err != nil {
		return "", err
	}
	return tag, nil
}

func cloneForBuild(ctx context.Context, url, token, ref, sha string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "pipeline-build-*")
	if err != nil {
		return "", nil, fmt.Errorf("create build dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	opts := &git.CloneOptions{URL: url}
	if token != "" {
		opts.Auth = &githttp.BasicAuth{Username: "x-access-token", Password: token}
	}
	if sha == "" && strings.HasPrefix(ref, "refs/heads/") {
		opts.ReferenceName = plumbing.ReferenceName(ref)
		opts.SingleBranch = true
		opts.Depth = 1
	}
	repo, err := git.PlainCloneContext(ctx, dir, false, opts)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("clone %q: %w", url, err)
	}
	if sha != "" {
		wt, err := repo.Worktree()
		if err == nil {
			err = wt.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(sha)})
		}
		if err != nil {
			cleanup()
			return "", nil, fmt.Errorf("checkout %s: %w", sha, err)
		}
	}
	return dir, cleanup, nil
}

func (a *pipelineActions) protectedCheck(ctx context.Context, svc *store.DesiredService) error {
	if svc.EnvironmentID == "" {
		return nil
	}
	env, err := a.db.GetEnvironment(ctx, svc.EnvironmentID)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load environment: %w", err)
	}
	if env.Protected {
		return fmt.Errorf("service %q is in protected environment %q; pipelines cannot deploy to protected environments", svc.Name, env.Name)
	}
	return nil
}

func (a *pipelineActions) push(ctx context.Context, service, image, strategy string, log func(string)) (store.DesiredService, error) {
	svc, err := a.db.GetDesiredService(ctx, service)
	if err != nil {
		return store.DesiredService{}, fmt.Errorf("load service %q: %w", service, err)
	}
	if err := a.protectedCheck(ctx, svc); err != nil {
		return store.DesiredService{}, err
	}
	if strategy != "" {
		switch strategy {
		case "rolling", "recreate", "blue-green":
			svc.Strategy = strategy
		default:
			return store.DesiredService{}, fmt.Errorf("unknown strategy %q", strategy)
		}
	}
	status, err := deploy.CheckFreeze(ctx, a.db, service, time.Now())
	if err != nil {
		return store.DesiredService{}, err
	}
	if status.Frozen {
		id, err := deploy.HoldDeploy(ctx, a.db, service, image, store.DeployAttemptSourceImage, deploy.HeldRequest{Kind: deploy.HeldKindImage, Image: image}, status)
		if err != nil {
			return store.DesiredService{}, err
		}
		log(fmt.Sprintf("%s is frozen, deploy %s held until the window ends", service, id))
		return store.DesiredService{}, &deploy.FrozenError{Status: status}
	}
	log(fmt.Sprintf("deploying %s to %s", service, image))
	return deploy.ImageTrigger{
		Store: a.db, Nudger: a.nudger, Resolver: a.resolver, Ordered: a.db, Sequencer: a.db,
		Logger: a.logger, Source: store.DeployAttemptSourceImage, Automatic: true,
	}.Deploy(ctx, *svc, image)
}

func (a *pipelineActions) Deploy(ctx context.Context, req pipeline.DeployRequest, log func(string)) error {
	updated, err := a.push(ctx, req.Service, req.Image, req.Strategy, log)
	if err != nil {
		return err
	}
	return a.waitReady(ctx, req.Service, updated.Image, req.Wait, log)
}

func (a *pipelineActions) waitReady(ctx context.Context, service, image string, wait time.Duration, log func(string)) error {
	deadline := time.Now().Add(wait)
	consecutive := 0
	last := "no status yet"
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
		conds, err := a.db.GetConditions(ctx, "application/"+service)
		if err != nil {
			return fmt.Errorf("read status: %w", err)
		}
		ready := len(conds) > 0
		for _, c := range conds {
			if c.Status == reconcile.ConditionFalse {
				ready = false
				last = c.Type + ": " + c.Reason
			}
		}
		svc, err := a.db.GetDesiredService(ctx, service)
		if err != nil || svc.Image != image {
			return fmt.Errorf("desired image changed while waiting (superseded by another deploy)")
		}
		if ready {
			consecutive++
		} else {
			consecutive = 0
		}
		if consecutive >= 2 {
			log(service + " is ready")
			return nil
		}
	}
	return fmt.Errorf("timed out after %s waiting for %s to become ready (last: %s)", wait, service, last)
}

func (a *pipelineActions) Promote(ctx context.Context, from, to string, log func(string)) (string, error) {
	src, err := a.db.GetDesiredService(ctx, from)
	if err != nil {
		return "", fmt.Errorf("load source service %q: %w", from, err)
	}
	if _, err := a.push(ctx, to, src.Image, "", log); err != nil {
		return "", err
	}
	return src.Image, nil
}

func (a *pipelineActions) Rollback(ctx context.Context, service string, log func(string)) (string, error) {
	svc, err := a.db.GetDesiredService(ctx, service)
	if err != nil {
		return "", fmt.Errorf("load service %q: %w", service, err)
	}
	attempts, err := a.db.ListDeployAttempts(ctx, service)
	if err != nil {
		return "", fmt.Errorf("list deploy history: %w", err)
	}
	image, ok := deploy.PreviousKnownGoodImage(attempts, svc.Image)
	if !ok {
		return "", fmt.Errorf("no previous successful deploy of %q to roll back to", service)
	}
	if _, err := a.push(ctx, service, image, "", log); err != nil {
		return "", err
	}
	return image, nil
}

// wireOIDC configures pipeline jobs' `oidc: {audience: ...}` support when
// an issuer URL is available, and leaves it unconfigured (a job that
// opts in fails with a clear per-job error) otherwise: minting a token
// with no real, externally reachable issuer URL would only mislead an
// operator into thinking cloud federation works when AWS, GCP, or Vault
// could never actually fetch the JWKS to verify it.
//
// It also resolves the address for the runtime "request a token for
// this audience" endpoint (a job needing more than one audience calls
// this instead of getting every combination pre-minted), returned so
// the caller can start the actual listener once the Engine exists. Empty
// means that endpoint is unavailable this run (oidc unconfigured, or the
// bridge gateway lookup failed): a job listing oidc.audiences then fails
// clearly instead of getting an unreachable URL.
func wireOIDC(ctx context.Context, cfg *pipeline.Config, apiRouter *api.Router, secretsManager *secrets.Manager, client *docker.Client, logger *slog.Logger) string {
	issuerURL := oidcIssuerURL()
	if issuerURL == "" {
		return ""
	}
	mgr, err := oidc.NewManager(oidc.Config{
		KeyStore:       secretsManager,
		IssuerURL:      issuerURL,
		TTL:            envDuration("APP_OIDC_TOKEN_TTL", 0),
		KeyRetireGrace: envDuration("APP_OIDC_KEY_RETIRE_GRACE", 0),
	})
	if err != nil {
		logger.Warn("pipeline: oidc not configured", slog.String("error", err.Error()))
		return ""
	}
	cfg.OIDCIssuer = func(ctx context.Context, req pipeline.OIDCTokenRequest) (string, error) {
		return mgr.IssueToken(ctx, oidc.TokenRequest{
			Audience: req.Audience, Subject: req.Subject, Repo: req.Repo, Ref: req.Ref, PipelineID: req.PipelineID,
		})
	}
	apiRouter.SetOIDCManager(mgr, issuerURL, oidcJWKSRateLimit())
	apiRouter.SetOIDCRotator(mgr)

	gateway, err := client.BridgeGatewayIP(ctx)
	if err != nil {
		logger.Warn("pipeline: oidc runtime token request endpoint not configured, jobs listing oidc.audiences will fail clearly instead of getting an unreachable URL",
			slog.String("error", err.Error()))
		return ""
	}
	addr := net.JoinHostPort(gateway, strconv.Itoa(oidcTokenRequestPort()))
	cfg.OIDCRequestURL = "http://" + addr + "/oidc/token"
	return addr
}

// defaultOIDCTokenRequestPort is where the runtime OIDC token request
// endpoint listens on the Docker bridge gateway IP, reachable from every
// job container attached to the default bridge network and nowhere
// else. An unprivileged port, no relation to any other service in this
// codebase. Overridable via APP_OIDC_TOKEN_REQUEST_PORT.
const defaultOIDCTokenRequestPort = 9095

func oidcTokenRequestPort() int {
	if v, err := strconv.Atoi(os.Getenv("APP_OIDC_TOKEN_REQUEST_PORT")); err == nil && v > 0 {
		return v
	}
	return defaultOIDCTokenRequestPort
}

// serveOIDCTokenRequests runs the runtime OIDC token request endpoint
// (internal/pipeline.Engine.OIDCTokenRequestHandler) on addr until ctx
// is cancelled. The bearer token a job presents, not network placement
// alone, is what actually scopes access to that job's own allowed
// audiences; addr being the bridge gateway IP only keeps it off any
// externally reachable interface.
func serveOIDCTokenRequests(ctx context.Context, addr string, engine *pipeline.Engine, logger *slog.Logger) {
	mux := http.NewServeMux()
	mux.Handle("/oidc/token", engine.OIDCTokenRequestHandler())
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("pipeline: oidc token request server stopped", slog.String("error", err.Error()))
	}
}

// oidcJWKSRateLimit reads APP_OIDC_JWKS_RATE_LIMIT_PER_MINUTE; 0 (the
// default, whether unset or invalid) tells api.Router to apply its own
// default. A dedicated reader rather than reusing envInt: every one of
// envInt's other call sites also passes 0, which makes golangci-lint's
// unparam check flag that parameter as dead across the whole file.
func oidcJWKSRateLimit() int {
	if v, err := strconv.Atoi(os.Getenv("APP_OIDC_JWKS_RATE_LIMIT_PER_MINUTE")); err == nil && v > 0 {
		return v
	}
	return 0
}

// oidcIssuerURL resolves the "iss" claim and JWKS base URL for pipeline
// OIDC tokens: APP_OIDC_ISSUER_URL wins, then APP_DASHBOARD_URL (already
// used for notification links, see alert_changes.go), then publicHost()
// over HTTPS. Empty means no configured issuer.
func oidcIssuerURL() string {
	if v := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_OIDC_ISSUER_URL")), "/"); v != "" {
		return v
	}
	if v := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_DASHBOARD_URL")), "/"); v != "" {
		return v
	}
	if host := publicHost(); host != "" {
		return "https://" + host
	}
	return ""
}

func (a *pipelineActions) Notify(ctx context.Context, app string, succeeded bool, message string) error {
	if a.notifier == nil {
		return errors.New("notifications are not configured on this control plane")
	}
	ev := alerting.DeployOutcome{AppName: app, Image: "pipeline: " + message, Succeeded: succeeded}
	if !succeeded {
		ev.Error = message
	}
	a.notifier.Dispatch(ctx, "service:"+app, ev)
	return nil
}
