// Package deployurls keeps an immutable hostname for each recent successful
// deploy: a retained service that runs that attempt's pinned image and sleeps
// when idle, so a past deployment can be opened without rolling back.
package deployurls

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Environment variables that tune deploy URLs.
const (
	// EnvKeep is how many recent successful deploys per app keep a URL; 0 turns the feature off.
	EnvKeep = "APP_DEPLOY_URL_KEEP"
	// EnvIdleMinutes is how long a retained deploy runs without traffic before it sleeps.
	EnvIdleMinutes = "APP_DEPLOY_URL_IDLE_MINUTES"
	// EnvInterval is how often the syncer looks for new deploys.
	EnvInterval = "APP_DEPLOY_URL_INTERVAL"
)

// Defaults for the variables above.
const (
	DefaultKeep        = 3
	DefaultIdleMinutes = 5
	DefaultInterval    = time.Minute
	keyLen             = 7
)

var previewName = regexp.MustCompile(`-pr-\d+$`)

// Config is the environment-driven tuning.
type Config struct {
	Keep        int
	IdleMinutes int
	Interval    time.Duration
}

// ConfigFromEnv reads the Env* variables, using defaults for anything missing or malformed.
func ConfigFromEnv(getenv func(string) string) Config {
	cfg := Config{Keep: DefaultKeep, IdleMinutes: DefaultIdleMinutes, Interval: DefaultInterval}
	if n, err := strconv.Atoi(strings.TrimSpace(getenv(EnvKeep))); err == nil && n >= 0 {
		cfg.Keep = n
	}
	if n, err := strconv.Atoi(strings.TrimSpace(getenv(EnvIdleMinutes))); err == nil && n > 0 {
		cfg.IdleMinutes = n
	}
	if d, err := time.ParseDuration(strings.TrimSpace(getenv(EnvInterval))); err == nil && d >= time.Second {
		cfg.Interval = d
	}
	return cfg
}

// Wanted is one deploy attempt that should have a URL.
type Wanted struct {
	Attempt     store.DeployAttempt
	Key         string
	ServiceName string
	Pin         *store.RollbackPin
	Ref         string
	// Live is true while the attempt is what the app runs now: the app's own URL serves it.
	Live bool
}

// ServiceName is the retained service for app and key; it doubles as the host label.
func ServiceName(app, key string) string { return app + "-" + key }

// Key is the short, DNS-safe identity of an attempt: the commit when it has one, else its id.
func Key(a store.DeployAttempt) string {
	src := a.CommitSHA
	if src == "" {
		src = strings.TrimPrefix(a.ID, "da_")
	}
	var b strings.Builder
	for _, r := range strings.ToLower(src) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
		if b.Len() == keyLen {
			break
		}
	}
	return b.String()
}

// Plan picks the newest keep successful attempts whose content can still be
// redeployed (a registry digest or a rollback pin). Two attempts of the same
// commit share one URL, the newest. Image GC is untouched: it only frees what
// no pin or running service holds.
func Plan(app store.DesiredService, attempts []store.DeployAttempt, pins []store.RollbackPin, keep int) []Wanted {
	if keep <= 0 {
		return nil
	}
	pinByAttempt := make(map[string]*store.RollbackPin, len(pins))
	for i := range pins {
		pinByAttempt[pins[i].DeployAttemptID] = &pins[i]
	}
	sorted := append([]store.DeployAttempt(nil), attempts...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].StartedAt.After(sorted[j].StartedAt) })

	seen := map[string]bool{}
	var out []Wanted
	for _, a := range sorted {
		if len(out) == keep {
			break
		}
		if a.Status != store.DeployAttemptStatusSucceeded {
			continue
		}
		key := Key(a)
		if key == "" || seen[key] {
			continue
		}
		pin := pinByAttempt[a.ID]
		ref, safe := deploy.RollbackRef(a, pin)
		if !safe {
			continue
		}
		seen[key] = true
		out = append(out, Wanted{Attempt: a, Key: key, ServiceName: ServiceName(app.Name, key), Pin: pin, Ref: ref, Live: a.Image == app.Image || ref == app.Image})
	}
	return out
}

// Clone is the retained service for w: the app's config restored to that
// attempt, running the pinned image, with no volumes, host ports, hooks or
// automatic rollback, so an old deploy cannot touch live data or fight the app.
func Clone(app store.DesiredService, w Wanted, host string) store.DesiredService {
	svc, ref, _ := deploy.RollbackDesired(app, w.Attempt, w.Pin)
	svc.Name = w.ServiceName
	svc.Image = ref
	svc.ImageID, svc.ImageIDRef = "", ""
	svc.Domains = []string{host}
	svc.Replicas = 1
	svc.Volumes, svc.BindMounts = nil, nil
	svc.HostPort, svc.PublishedPorts = nil, nil
	svc.Hooks = nil
	svc.Suspended = false
	svc.AutoRollbackOnCrashloop, svc.AutoRollbackOnSLOBurn = false, ""
	svc.ExecEnabled = false
	return svc
}

// Store is what the syncer needs; *store.DB satisfies it.
type Store interface {
	ListDesiredServices(ctx context.Context) ([]store.DesiredService, error)
	SaveDesiredService(ctx context.Context, svc store.DesiredService) error
	ListDeployAttempts(ctx context.Context, serviceName string) ([]store.DeployAttempt, error)
	ListRollbackPins(ctx context.Context, serviceName string) ([]store.RollbackPin, error)
	SetAppSleepIdle(ctx context.Context, serviceName string, minutes int, now time.Time) error
	SetAppSleepHold(ctx context.Context, serviceName string, hold bool, now time.Time) error
	ListDeployURLs(ctx context.Context, appName string) ([]store.DeployURL, error)
	SaveDeployURL(ctx context.Context, u store.DeployURL) error
	DeleteDeployURL(ctx context.Context, serviceName string) error
}

// Syncer reconciles retained deploy services against each app's recent deploys.
type Syncer struct {
	Store  Store
	Config Config
	// HostFor returns the hostname for a retained service, or "" when none can be formed.
	HostFor func(ctx context.Context, serviceName string) string
	// Remove deletes a retained service and tears its containers down.
	Remove func(ctx context.Context, serviceName string) error
	// Nudge asks the reconcilers to run now.
	Nudge  func()
	Now    func() time.Time
	Logger *slog.Logger

	existing map[string]bool
}

func (s *Syncer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Sync runs one pass. A failure on one app is logged and does not stop the rest.
func (s *Syncer) Sync(ctx context.Context) error {
	services, err := s.Store.ListDesiredServices(ctx)
	if err != nil {
		return fmt.Errorf("deployurls: list services: %w", err)
	}
	rows, err := s.Store.ListDeployURLs(ctx, "")
	if err != nil {
		return fmt.Errorf("deployurls: list rows: %w", err)
	}
	byApp := map[string]map[string]store.DeployURL{}
	retained := map[string]bool{}
	for _, r := range rows {
		retained[r.ServiceName] = true
		if byApp[r.AppName] == nil {
			byApp[r.AppName] = map[string]store.DeployURL{}
		}
		byApp[r.AppName][r.ServiceName] = r
	}
	apps := map[string]bool{}
	existing := make(map[string]bool, len(services))
	for _, svc := range services {
		existing[svc.Name] = true
	}
	s.existing = existing
	changed := false
	for _, svc := range services {
		if retained[svc.Name] || store.IsCanaryService(svc.Name) || previewName.MatchString(svc.Name) {
			continue
		}
		apps[svc.Name] = true
		ok, err := s.syncApp(ctx, svc, byApp[svc.Name])
		if err != nil {
			s.Logger.WarnContext(ctx, "deploy urls: sync failed", slog.String("app", svc.Name), slog.String("error", err.Error()))
		}
		changed = changed || ok
	}
	for _, r := range rows {
		if !apps[r.AppName] && s.removeRow(ctx, r) {
			changed = true
		}
	}
	if changed && s.Nudge != nil {
		s.Nudge()
	}
	return nil
}

func (s *Syncer) removeRow(ctx context.Context, r store.DeployURL) bool {
	if err := s.Remove(ctx, r.ServiceName); err != nil && !errors.Is(err, store.ErrServiceNotFound) {
		s.Logger.WarnContext(ctx, "deploy urls: remove retained service failed", slog.String("service", r.ServiceName), slog.String("error", err.Error()))
		return false
	}
	if err := s.Store.DeleteDeployURL(ctx, r.ServiceName); err != nil {
		s.Logger.WarnContext(ctx, "deploy urls: delete row failed", slog.String("service", r.ServiceName), slog.String("error", err.Error()))
		return false
	}
	return true
}

func (s *Syncer) syncApp(ctx context.Context, app store.DesiredService, have map[string]store.DeployURL) (bool, error) {
	var wanted []Wanted
	if s.Config.Keep > 0 && !app.Suspended {
		attempts, err := s.Store.ListDeployAttempts(ctx, app.Name)
		if err != nil {
			return false, fmt.Errorf("list attempts: %w", err)
		}
		pins, err := s.Store.ListRollbackPins(ctx, app.Name)
		if err != nil {
			return false, fmt.Errorf("list pins: %w", err)
		}
		wanted = Plan(app, attempts, pins, s.Config.Keep)
	}
	want := map[string]bool{}
	changed := false
	for _, w := range wanted {
		want[w.ServiceName] = true
		if _, ok := have[w.ServiceName]; ok || w.Live {
			continue
		}
		if s.existing[w.ServiceName] {
			continue
		}
		host := ""
		if s.HostFor != nil {
			host = s.HostFor(ctx, w.ServiceName)
		}
		if host == "" {
			continue
		}
		if err := s.create(ctx, app, w, host); err != nil {
			s.Logger.WarnContext(ctx, "deploy urls: create failed", slog.String("service", w.ServiceName), slog.String("error", err.Error()))
			continue
		}
		changed = true
	}
	for name, row := range have {
		if !want[name] || (wantedLive(wanted, name)) {
			if s.removeRow(ctx, row) {
				changed = true
			}
		}
	}
	return changed, nil
}

func wantedLive(wanted []Wanted, name string) bool {
	for _, w := range wanted {
		if w.ServiceName == name {
			return w.Live
		}
	}
	return false
}

func (s *Syncer) create(ctx context.Context, app store.DesiredService, w Wanted, host string) error {
	if err := s.Store.SaveDesiredService(ctx, Clone(app, w, host)); err != nil {
		return fmt.Errorf("save service: %w", err)
	}
	now := s.now()
	if err := s.Store.SetAppSleepIdle(ctx, w.ServiceName, s.Config.IdleMinutes, now); err != nil {
		return fmt.Errorf("enable sleep: %w", err)
	}
	if err := s.Store.SetAppSleepHold(ctx, w.ServiceName, true, now); err != nil {
		return fmt.Errorf("hold requests: %w", err)
	}
	return s.Store.SaveDeployURL(ctx, store.DeployURL{ServiceName: w.ServiceName, AppName: app.Name, AttemptID: w.Attempt.ID, Host: host, CreatedAt: now})
}

// Run syncs every Config.Interval until ctx ends.
func (s *Syncer) Run(ctx context.Context) error {
	if !s.Config.Enabled() {
		if err := s.Sync(ctx); err != nil {
			s.Logger.WarnContext(ctx, "deploy urls: cleanup pass failed", slog.String("error", err.Error()))
		}
		return nil
	}
	t := time.NewTicker(s.Config.Interval)
	defer t.Stop()
	for {
		if err := s.Sync(ctx); err != nil {
			s.Logger.WarnContext(ctx, "deploy urls: pass failed", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// Enabled reports whether any URLs are kept.
func (c Config) Enabled() bool { return c.Keep > 0 }

// FromProcessEnv is ConfigFromEnv over the process environment.
func FromProcessEnv() Config { return ConfigFromEnv(os.Getenv) }
