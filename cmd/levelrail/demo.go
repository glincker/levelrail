package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/glincker/levelrail/internal/alerting"
	"github.com/glincker/levelrail/internal/api"
	"github.com/glincker/levelrail/internal/backup"
	"github.com/glincker/levelrail/internal/brand"
	"github.com/glincker/levelrail/internal/demo"
	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/telemetry"
	"github.com/glincker/levelrail/internal/version"
	"github.com/glincker/levelrail/web"
)

const (
	demoDefaultAddr  = "127.0.0.1:8080"
	demoSessionTTL   = 2 * time.Hour
	demoResetHourUTC = 4
	// demoEnvResetInterval overrides the daily reset time with a fixed cadence,
	// so the banner matches a host that re-seeds more often.
	demoEnvResetInterval = "APP_DEMO_RESET_INTERVAL"
	// demoEnvPublicURL is the public https origin used in share tags.
	demoEnvPublicURL     = "APP_DEMO_PUBLIC_URL"
	demoDefaultPublicURL = "https://demo.levelrail.com"

	demoShareTitle       = "Levelrail live demo: self-hosted deploys, no signup"
	demoShareDescription = "Click through a self-hosted deploy platform: deploys, rollbacks, databases and domains. Read only, fictional data, nothing to install."
	demoShareImageAlt    = "Levelrail live demo: try the dashboard with fictional data"
	demoShareImageWidth  = 1200
	demoShareImageHeight = 630
	demoMinResetInterval = time.Minute
	// Link preview bots may read the page so shared links show a card. Search
	// engines stay out: the catch-all rule and the noindex header still apply.
	demoRobotsTxt = "User-agent: Redditbot\nUser-agent: Twitterbot\nUser-agent: facebookexternalhit\n" +
		"User-agent: Slackbot\nUser-agent: LinkedInBot\nUser-agent: Discordbot\nAllow: /\n\n" +
		"User-agent: *\nDisallow: /\n"
)

// runDemo dispatches `demo seed` and `demo serve`. The demo is a separate
// entrypoint on purpose: the ordinary run() path never reads APP_DEMO_MODE.
func runDemo(ctx context.Context, logger *slog.Logger, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: demo <seed|serve|check>")
	}
	switch args[0] {
	case "seed":
		return runDemoSeed(ctx, logger, args[1:], stdout)
	case "serve":
		return runDemoServe(ctx, logger)
	case "check":
		return runDemoCheck(ctx, stdout)
	default:
		return fmt.Errorf("unknown demo command %q (want seed, serve or check)", args[0])
	}
}

func demoDomainSuffix(b *brand.Brand) string {
	if b.Domain == "" {
		return ""
	}
	return "demo." + b.Domain
}

func runDemoSeed(ctx context.Context, logger *slog.Logger, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("demo seed", flag.ContinueOnError)
	seed := fs.Int64("seed", 1, "deterministic seed")
	anchorRaw := fs.String("anchor", "now", "RFC 3339 anchor time, or now")
	scenarioRaw := fs.String("scenario", demo.ScenarioIncident, "storyline: incident or calm")
	if err := fs.Parse(args); err != nil {
		return err
	}
	scenario, err := demo.ParseScenario(*scenarioRaw)
	if err != nil {
		return err
	}
	anchor := time.Now().UTC().Truncate(time.Minute)
	if *anchorRaw != "now" {
		t, err := time.Parse(time.RFC3339, *anchorRaw)
		if err != nil {
			return fmt.Errorf("demo seed: invalid --anchor: %w", err)
		}
		anchor = t
	}
	b, err := loadBrand()
	if err != nil {
		return err
	}
	db, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			logger.Error("closing store", slog.String("error", cerr.Error()))
		}
	}()
	telemetryDB, err := openTelemetryStore(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = telemetryDB.Close() }()
	alertingDB, err := openAlertingStore(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = alertingDB.Close() }()
	m, err := demo.SeedAll(ctx, demo.Stores{DB: db, Telemetry: telemetryDB, Alerting: alertingDB},
		demo.Options{Seed: *seed, Anchor: anchor, DomainSuffix: demoDomainSuffix(b), Scenario: scenario, DataDir: dataDirFromEnv()})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "seeded demo data: %d services, %d databases, %d nodes\n", len(m.Services), len(m.Databases), len(m.Nodes))
	return nil
}

func runDemoServe(ctx context.Context, logger *slog.Logger) error {
	if err := demo.CheckEnvironment(os.Getenv); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	b, err := loadBrand()
	if err != nil {
		return err
	}
	db, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := demo.CheckStore(ctx, db); err != nil {
		return err
	}
	telemetryDB, err := openTelemetryStore(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = telemetryDB.Close() }()
	alertingDB, err := openAlertingStore(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = alertingDB.Close() }()

	broadcaster := telemetry.NewLogBroadcaster()
	handler, err := buildDemoHandler(ctx, logger, b, db, telemetryDB, alertingDB, broadcaster)
	if err != nil {
		return err
	}
	svcs, err := db.ListDesiredServices(ctx)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(svcs))
	for _, s := range svcs {
		names = append(names, s.Name)
	}
	gen := &demo.Generator{Services: names, Logs: broadcaster, Samples: telemetryDB, Seed: time.Now().UnixNano()}
	go gen.Run(ctx)

	addr := demo.ResolveAddr(os.Getenv, demo.InContainer, demoDefaultAddr)
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	logger.Info("demo instance listening", slog.String("addr", addr))
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// buildDemoHandler wires the Router with only what a seeded, Docker-free
// instance needs: no engine, collector, builder, ingress, agent or exec runtime.
func buildDemoHandler(ctx context.Context, logger *slog.Logger, b *brand.Brand, db *store.DB, telemetryDB *telemetry.DB, alertingDB *alerting.DB, broadcaster *telemetry.LogBroadcaster) (http.Handler, error) {
	fakes := demo.NewDocker(db, time.Now)
	cfg := api.DemoConfig{
		VisitorUserID: demo.VisitorUserID, Username: demo.VisitorUsername, Password: demo.VisitorPassword,
		ResetsAt: nextDemoReset, RepoURL: b.RepoURL, DocsURL: b.DocsURL, Version: version.Version,
	}
	if b.Domain != "" {
		cfg.InstallCommand = fmt.Sprintf("curl -fsSL https://%s/install.sh | sh", b.Domain)
	}
	opts := []api.Option{
		api.WithDemoMode(cfg),
		api.WithSessionTTL(demoSessionTTL),
		api.WithTelemetryQuerier(telemetry.NewLocalFederator(telemetryDB)),
		api.WithDeployLogQuerier(telemetryDB),
		api.WithLogBroadcaster(broadcaster),
		api.WithAccessSource(demo.Access{}),
		api.WithAlertRules(alertingDB),
		api.WithAlertDefaults(alertingDB),
		api.WithBackupProtection(&backup.Protection{Store: db}, db),
		api.WithOrphanedVolumeManager(demo.Volumes{}),
		api.WithAlertNoise(alertingDB),
		api.WithInbox(alertingDB),
		api.WithStatusPage(db, statusPageConfig(logger), statusPageRateLimit(logger)),
		api.WithAnalysisReports(db),
		api.WithDBPinger(db),
		api.WithDockerPinger(fakes),
		api.WithImageLister(fakes),
		api.WithContainerLister(fakes),
		api.WithDockerDiskUsager(fakes),
		api.WithDataDir(dataDirFromEnv()),
	}
	authOpts, err := authEngineOptions(ctx, logger, b, db.DB, nil)
	if err != nil {
		return nil, fmt.Errorf("demo: auth engine setup failed: %w", err)
	}
	opts = append(opts, authOpts...)
	rt := api.NewRouter(logger, b, db, opts...)
	// Without a sampler the public status page reads Unknown for every component.
	go func() { _ = rt.RunStatusPageSampler(ctx) }()
	head := demoHeadOptions(logger, os.Getenv(demoEnvPublicURL))
	// The embedded dashboard sits outside the API chain, so it needs the widened policy too.
	head.CSP = rt.DemoCSP(web.DashboardCSP)
	head.GTMID = rt.DemoGTMID()
	mux := composeMux(rt.Handler(), nil, demoWebHandler(b, head))
	// Registered on the mux so it wins over the static handler's catch-all.
	mux.HandleFunc("/theme-init.js", serveDemoThemeInit)
	mux.HandleFunc(web.GTMInitPath, serveGTMInit)
	return demoStaticHeaders(mux), nil
}

// demoWebHandler is a seam so tests can serve a fixture shell without embedweb.
var demoWebHandler = web.HandlerWithHead

// demoHeadOptions builds the demo's share tags. An unset or invalid public
// URL falls back to the default; an invalid one logs a warning.
func demoHeadOptions(logger *slog.Logger, raw string) web.HeadOptions {
	origin := demoDefaultPublicURL
	if raw != "" {
		if o, ok := absoluteHTTPSOrigin(raw); ok {
			origin = o
		} else {
			logger.Warn("ignoring invalid "+demoEnvPublicURL+": must be an absolute https URL", slog.String("env", demoEnvPublicURL))
		}
	}
	return web.HeadOptions{
		Title:       demoShareTitle,
		Description: demoShareDescription,
		URL:         origin + "/",
		ImageURL:    origin + "/demo-og.png",
		ImageAlt:    demoShareImageAlt,
		ImageWidth:  demoShareImageWidth,
		ImageHeight: demoShareImageHeight,
	}
}

// absoluteHTTPSOrigin returns scheme://host for an absolute https URL.
func absoluteHTTPSOrigin(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", false
	}
	return "https://" + u.Host, true
}

// demoThemeInit mirrors web/public/theme-init.js except an unset choice is
// light; the data-theme-default marker makes ThemeProvider agree.
const demoThemeInit = `;(function () {
  document.documentElement.setAttribute('data-theme-default', 'light')
  try {
    var stored = localStorage.getItem('levelrail-theme')
    var isDark =
      stored === 'system'
        ? window.matchMedia('(prefers-color-scheme: dark)').matches
        : stored === 'dark'
    document.documentElement.classList.toggle(
      'dark',
      isDark || stored === 'oled',
    )
    document.documentElement.classList.toggle('oled', stored === 'oled')
    try {
      var saved = JSON.parse(localStorage.getItem('levelrail-appearance-v1'))
      if (saved && saved.look === 'restrained') {
        document.documentElement.setAttribute('data-look', 'restrained')
      }
      if (saved && saved.backdrop === 'plain') {
        document.documentElement.setAttribute('data-backdrop', 'plain')
      }
    } catch (e) {}
    if (stored === 'oled') {
      document.write(
        '<link rel="stylesheet" id="levelrail-oled-css" href="/glinr.oled.css">',
      )
    }
  } catch (e) {}
})()
`

func serveDemoThemeInit(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.WriteString(w, demoThemeInit)
}

func serveGTMInit(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.WriteString(w, web.GTMInitScript)
}

// demoStaticHeaders marks every response noindex and serves a disallow-all
// robots.txt, covering the static shell the API gate never sees.
func demoStaticHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = io.WriteString(w, demoRobotsTxt)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func nextDemoReset() time.Time {
	return nextDemoResetAt(time.Now().UTC(), os.Getenv(demoEnvResetInterval))
}

// nextDemoResetAt returns the next reset after now: the next multiple of the
// configured interval (UTC aligned), or the daily default when it is unset or invalid.
func nextDemoResetAt(now time.Time, interval string) time.Time {
	if d, err := time.ParseDuration(interval); err == nil && d >= demoMinResetInterval {
		return now.Truncate(d).Add(d)
	}
	next := time.Date(now.Year(), now.Month(), now.Day(), demoResetHourUTC, 0, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// runDemoCheck lints the seeded data dir and fails when any record is
// dangling, secret-looking, off the example domains or in the future.
func runDemoCheck(ctx context.Context, stdout io.Writer) error {
	b, err := loadBrand()
	if err != nil {
		return err
	}
	db, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	telemetryDB, err := openTelemetryStore(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = telemetryDB.Close() }()
	alertingDB, err := openAlertingStore(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = alertingDB.Close() }()
	findings, err := demo.Lint(ctx, demo.Stores{DB: db, Telemetry: telemetryDB, Alerting: alertingDB}, demo.LintOptions{DomainSuffix: demoDomainSuffix(b)})
	if err != nil {
		return err
	}
	for _, f := range findings {
		_, _ = fmt.Fprintln(stdout, f.String())
	}
	if len(findings) > 0 {
		return fmt.Errorf("demo check: %d findings", len(findings))
	}
	_, _ = fmt.Fprintln(stdout, "demo check: seed data is clean")
	return nil
}
