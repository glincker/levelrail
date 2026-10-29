package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/autoscale"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/changelog"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/localmode"
	"github.com/GLINCKER/levelrail/internal/pkgregistry"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	registryreconcile "github.com/GLINCKER/levelrail/internal/reconcile/registry"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/staticroutes"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/supplychain"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// Turns a missing brand.yaml into an actionable message, since it would otherwise read as an opaque crash.
// brand.Load itself stays strict about its input.
func loadBrand() (*brand.Brand, error) {
	path := os.Getenv("APP_BRAND_FILE")
	if path == "" {
		path = defaultBrandFile
	}
	b, err := brand.Load(path)
	if err != nil && os.IsNotExist(errors.Unwrap(err)) && localmode.Enabled(os.Getenv) {
		return brand.LoadEmbedded()
	}
	if err != nil && os.IsNotExist(errors.Unwrap(err)) {
		return nil, fmt.Errorf("%w (running via install.sh sets this up automatically; running the binary directly needs either a brand.yaml file at %q or APP_BRAND_FILE pointing at one)", err, path)
	}
	return b, err
}

// A missing CHANGELOG.md is not fatal: the what's-new panel is simply empty.
func loadChangelog(logger *slog.Logger) []changelog.Entry {
	path := os.Getenv("APP_CHANGELOG_FILE")
	content, found, err := changelog.ReadFile(path, defaultChangelogFile)
	if err != nil {
		logger.Warn("read changelog file failed", slog.String("error", err.Error()))
		return nil
	}
	if !found {
		return nil
	}
	return changelog.Parse(content)
}

func loadGitHubAppManifestConfig() (githubapp.ManifestConfig, error) {
	path := os.Getenv("APP_GITHUB_APP_MANIFEST_FILE")
	if path == "" {
		path = defaultGitHubAppManifestFile
	}
	return githubapp.LoadManifestConfig(path)
}

// devFixturesFile resolves the path MaybeSeedDevFixturesFromFile reads,
// the same default-plus-env-override shape as loadBrand above.
func devFixturesFile() string {
	path := os.Getenv("APP_DEV_FIXTURES_FILE")
	if path == "" {
		path = defaultDevFixturesFile
	}
	return path
}

// loadSecretsManager builds a secrets.Manager from APP_MASTER_KEY, or a
// key persisted in (or first generated into) dataDir. masterKeyFilePath
// is "" for an env-sourced key, so rotation knows it cannot persist it.
func loadSecretsManager(db *store.DB, dataDir string, opts ...secrets.ManagerOption) (mgr *secrets.Manager, masterKeyFilePath string, err error) {
	if serialized := os.Getenv("APP_MASTER_KEY"); serialized != "" {
		plain, err := openMasterKeyMaterial(context.Background(), serialized, os.Getenv)
		if err != nil {
			return nil, "", fmt.Errorf("open master key from APP_MASTER_KEY: %w", err)
		}
		mk, err := secrets.LoadMasterKey(plain)
		if err != nil {
			return nil, "", fmt.Errorf("load master key from APP_MASTER_KEY: %w", err)
		}
		return secrets.NewManager(db, mk, opts...), "", nil
	}

	mk, keyPath, err := loadOrGenerateMasterKey(dataDir)
	if err != nil {
		return nil, "", err
	}
	mgr = secrets.NewManager(db, mk, opts...)
	promoted, err := mgr.RecoverPendingMasterKey(context.Background(), keyPath, func(raw string) (string, error) {
		return openMasterKeyMaterial(context.Background(), raw, os.Getenv)
	})
	if err != nil {
		return nil, "", fmt.Errorf("recover interrupted master key rotation: %w", err)
	}
	if promoted {
		slog.Info("master key rotation was interrupted; completed it from the staged key file")
	}
	return mgr, keyPath, nil
}

// loadOrGenerateMasterKey loads a previously persisted master key from
// dataDir, or generates and persists a fresh one if none exists yet.
func loadOrGenerateMasterKey(dataDir string) (mk *secrets.MasterKey, keyPath string, err error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil { //nolint:gosec // operator-controlled startup config, not user input
		return nil, "", fmt.Errorf("create data dir for master key: %w", err)
	}
	keyPath = filepath.Join(dataDir, masterKeyFilename)
	if custom := strings.TrimSpace(os.Getenv(envMasterKeyFile)); custom != "" {
		if !filepath.IsAbs(custom) {
			return nil, "", fmt.Errorf("%s must be an absolute path, got %q", envMasterKeyFile, custom)
		}
		keyPath = custom
		if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil { //nolint:gosec // operator-set absolute path from startup config
			return nil, "", fmt.Errorf("create directory for %s: %w", envMasterKeyFile, err)
		}
		if st, err := os.Stat(keyPath); err == nil && st.Mode().Perm()&0o077 != 0 { //nolint:gosec // operator-set absolute path from startup config
			return nil, "", fmt.Errorf("%s %s has mode %o, want 0600: run chmod 600 on it", envMasterKeyFile, keyPath, st.Mode().Perm())
		}
	}

	serialized, readErr := os.ReadFile(keyPath) //nolint:gosec // operator-controlled data directory path, not user input
	switch {
	case readErr == nil:
		plain, err := openMasterKeyMaterial(context.Background(), string(serialized), os.Getenv)
		if err != nil {
			return nil, "", fmt.Errorf("open persisted master key: %w", err)
		}
		mk, err := secrets.LoadMasterKey(plain)
		if err != nil {
			return nil, "", fmt.Errorf("load persisted master key: %w", err)
		}
		return mk, keyPath, nil
	case !os.IsNotExist(readErr):
		// Never treat an I/O failure as 'no key yet': regenerating would orphan every secret already wrapped.
		return nil, "", fmt.Errorf("read persisted master key at %s: %w", keyPath, readErr)
	}

	mk, err = secrets.GenerateMasterKey()
	if err != nil {
		return nil, "", fmt.Errorf("generate master key: %w", err)
	}
	sealed, err := sealMasterKeyMaterial(context.Background(), mk.String(), os.Getenv)
	if err != nil {
		return nil, "", fmt.Errorf("wrap new master key: %w", err)
	}
	if err := os.WriteFile(keyPath, []byte(sealed), 0o600); err != nil { //nolint:gosec // operator-controlled data directory path, not user input
		return nil, "", fmt.Errorf("persist master key: %w", err)
	}
	return mk, keyPath, nil
}

// Shared by the git webhook and the manual build trigger, so both use one BuildKit connection.
// Returns an error rather than a partially wired builder; the caller treats it as non-fatal.
func loadBuilder(ctx context.Context, logger *slog.Logger, db *store.DB, telemetryDB *telemetry.DB, secretsManager *secrets.Manager, agentRegistry *agent.Registry, supplyChain *supplychain.Service, guard dockerguard.Booted) (*deploy.Pipeline, *build.Limiter, func() error, error) {
	rawDockerCli, err := rawDockerClient(guard.Host)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("new docker client for buildkit: %w", err)
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	buildClient, err := build.NewClient(connectCtx, rawDockerCli, buildCacheOptions(ctx, db, logger)...)
	cancel()
	if err != nil {
		_ = rawDockerCli.Close()
		return nil, nil, nil, fmt.Errorf("connect to buildkit: %w", err)
	}

	closer := func() error {
		buildErr := buildClient.Close()
		dockerErr := rawDockerCli.Close()
		if buildErr != nil {
			return buildErr
		}
		return dockerErr
	}

	// Static sites are copied here, not served from the checkout, which the webhook removes on return.
	dataDir := os.Getenv("APP_DATA_DIR")
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	staticSitesDir := filepath.Join(dataDir, "static")

	deployOpts := []deploy.Option{
		deploy.WithBuildMetricsRecorder(telemetryDB),
		deploy.WithLogger(logger),
		deploy.WithStaticSiteStore(db),
		deploy.WithStaticRootDir(staticSitesDir),
		deploy.WithStaticRouteLimits(staticroutes.LimitsFromEnv(os.LookupEnv)),
		deploy.WithStaticExtractor(build.NewImageExtractor(rawDockerCli, build.StaticMaxBytesFromEnv(os.LookupEnv))),
		deploy.WithStaticBuildImage(os.Getenv(deploy.EnvStaticBuildImage)),
		deploy.WithStaticPublishRecorder(db),
		deploy.WithAppStore(db),
		deploy.WithPublishLocks(db),
		deploy.WithVaultConfigChecker(db),
		deploy.WithLoadBalancerStore(db),
		deploy.WithSpendCapStore(db),
		deploy.WithAutoscaleStore(db, autoscale.CeilingFromEnv(autoscale.OSEnv)),
		deploy.WithCanaryPolicyStore(db),
		deploy.WithRolloutPolicyStore(db),
		deploy.WithJobStore(db),
		deploy.WithOrderedStore(db),
		deploy.WithAttemptRecorder(db),
	}
	deployOpts = append(deployOpts, digestResolverOptions(logger, db, secretsManager, guard)...)
	deployOpts = append(deployOpts, supplyChainDeployOptions(supplyChain)...)
	deployOpts = append(deployOpts, secretScanDeployOptions(db, logger)...)
	deployOpts = append(deployOpts, spendGuardDeployOptions(db, logger)...)
	if secretsManager != nil {
		deployOpts = append(deployOpts, deploy.WithSecretChecker(secretsManager), deploy.WithBuildSecretValues(secretsManager), deploy.WithBuildCache(newBuildCache(logger, db, secretsManager)),
			deploy.WithBuildSecretSource(&pkgregistry.Resolver{Store: db, Secrets: secretsManager}))
	}

	router := build.NewRouter(buildClient, buildNodeSource(db, agentRegistry), agent.NewBuildDispatcher(agentRegistry), build.WithRouterLogger(logger))
	limiter := build.NewLimiter(router, api.BuildLimitsSource(db, build.LimitsFromEnv(os.LookupEnv)),
		build.WithRunRecorder(api.BuildRunRecorder(db)), build.WithLimiterLogger(logger),
		build.WithNodeResolver(router.SelectedNode), build.WithImageSizer(router.ImageSize),
		build.WithCgroupProvisioner(build.NewCgroupProvisioner(os.Getenv(build.EnvBuildCgroupRoot))))
	return deploy.New(limiter, db, deployOpts...), limiter, closer, nil
}

// Requires every input, so any missing one returns an error rather than a partially wired handler.
func loadWebhookHandler(logger *slog.Logger, b *brand.Brand, db *store.DB, recorder *deploylog.Recorder, notifier *alerting.DeployDispatcher, pipeline *deploy.Pipeline, nudger *reconcile.Engine) (http.Handler, error) {
	if pipeline == nil {
		return nil, fmt.Errorf("no builder available (see the earlier \"builder not configured\" warning)")
	}

	repoURL := os.Getenv("APP_GIT_REPO_URL")
	webhookSecret := os.Getenv("APP_WEBHOOK_SECRET")
	imageRepo := os.Getenv("APP_IMAGE_REPO")
	if repoURL == "" || webhookSecret == "" || imageRepo == "" {
		return nil, fmt.Errorf("APP_GIT_REPO_URL, APP_WEBHOOK_SECRET, and APP_IMAGE_REPO must all be set")
	}

	specDir := os.Getenv("APP_SPEC_DIR")
	if specDir == "" {
		specDir = "."
	}
	specPath, err := spec.DiscoverPath(specDir, strings.ToLower(b.BinaryName))
	if err != nil {
		return nil, fmt.Errorf("discover app spec: %w", err)
	}
	// Operator-controlled path, not request input, so the gosec file-path warning does not apply.
	data, err := os.ReadFile(specPath) //nolint:gosec // operator-controlled startup config, not user input
	if err != nil {
		return nil, fmt.Errorf("read app spec %s: %w", specPath, err)
	}
	parsed, err := spec.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse app spec %s: %w", specPath, err)
	}

	serviceName := os.Getenv("APP_SERVICE_NAME")
	if serviceName == "" {
		if len(parsed.Services) != 1 {
			return nil, fmt.Errorf("APP_SERVICE_NAME must be set: app spec %s declares %d services, not exactly 1", specPath, len(parsed.Services))
		}
		for name := range parsed.Services {
			serviceName = name
		}
	}
	svc, ok := parsed.Services[serviceName]
	if !ok {
		return nil, fmt.Errorf("service %q not found in app spec %s", serviceName, specPath)
	}

	cfg := webhook.Config{
		Secret:      []byte(webhookSecret),
		RepoURL:     repoURL,
		Branch:      os.Getenv("APP_GIT_BRANCH"),
		ServiceName: serviceName,
		Service:     svc,
		ImageRepo:   imageRepo,
	}
	return webhook.New(cfg, pipeline, db, recorder, notifier, nudger, logger), nil
}

// The built-in registry is used as the cache automatically unless APP_BUILD_CACHE_REGISTRY is set.
// Changing either takes a restart, since these are read once at startup.
func buildCacheOptions(ctx context.Context, db *store.DB, logger *slog.Logger) []build.Option {
	var opts []build.Option
	if dir := os.Getenv("APP_BUILD_CACHE_DIR"); dir != "" {
		opts = append(opts, build.WithCacheDir(dir))
	}
	if ref := os.Getenv("APP_BUILD_CACHE_REGISTRY"); ref != "" {
		opts = append(opts, build.WithCacheRegistry(ref))
		if os.Getenv("APP_BUILD_CACHE_REGISTRY_INSECURE") == "true" {
			opts = append(opts, build.WithCacheRegistryInsecure())
		}
		return opts
	}

	settings, err := db.GetRegistrySettings(ctx)
	if err != nil {
		logger.Warn("build cache: get registry settings failed, continuing without a cache backend", slog.String("error", err.Error()))
		return opts
	}
	if settings.Enabled && settings.Host != "" {
		opts = append(opts, build.WithCacheRegistry(settings.Host+"/buildcache"), build.WithCacheRegistryInsecure())
	}
	return opts
}

// Fixed loopback port: registryreconcile.HostPort is fixed, so there is nothing to inspect.
func registryDialAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", registryreconcile.HostPort)
}

// Checked per build, so marking a node build-capable takes effect without a restart.
func buildNodeSource(db *store.DB, agentRegistry *agent.Registry) build.NodeSource {
	return func(ctx context.Context) ([]build.NodeInfo, error) {
		nodes, err := db.ListNodes(ctx)
		if err != nil {
			return nil, fmt.Errorf("list nodes: %w", err)
		}

		infos := make([]build.NodeInfo, 0, len(nodes))
		for _, n := range nodes {
			online := false
			if _, err := agentRegistry.Get(n.ID); err == nil {
				online = true
			}
			infos = append(infos, build.NodeInfo{
				ID:                    n.ID,
				AcceptsBuildWorkloads: n.AcceptsBuildWorkloads,
				Online:                online,
			})
		}
		return infos, nil
	}
}
