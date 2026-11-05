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
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Environment variables that tune deploy URLs.
const (
	// EnvKeep is how many recent successful deploys per app keep a URL; 0 turns the feature off.
	EnvKeep = "APP_DEPLOY_URL_KEEP"
	// EnvIdleMinutes is how long a retained deploy runs without traffic before it sleeps.
	EnvIdleMinutes = "APP_DEPLOY_URL_IDLE_MINUTES"
	// EnvInterval is how often the syncer looks for new deploys.
	EnvInterval = "APP_DEPLOY_URL_INTERVAL"
	// EnvMaxCopies caps the retained copies of older deploys per app, the live deploy excluded.
	EnvMaxCopies = "APP_DEPLOY_URL_MAX_COPIES"
	// EnvMinFreeMemoryMB is the host available memory below which no new copy is created; 0 turns the check off.
	EnvMinFreeMemoryMB = "APP_DEPLOY_URL_MIN_FREE_MEMORY_MB"
	// EnvMaxMemoryPercent skips an app whose memory limit exceeds this share of host RAM; 0 turns the check off.
	EnvMaxMemoryPercent = "APP_DEPLOY_URL_MAX_MEMORY_PERCENT"
	// EnvCopies seeds the copies mode (off, private, public) until a stored value overrides it.
	EnvCopies = "APP_DEPLOY_URL_COPIES"
)

// Copies modes. Private keeps each copy asleep with no hostname, so no route or
// certificate exists for it until the mode is set to public.
const (
	ModeOff     = "off"
	ModePrivate = "private"
	ModePublic  = "public"
	// DefaultMode is what an install gets when neither a stored value nor the env seed is set.
	DefaultMode = ModePrivate
)

// ParseMode normalizes s to a copies mode; ok is false for anything else, including "".
func ParseMode(s string) (mode string, ok bool) {
	switch m := strings.ToLower(strings.TrimSpace(s)); m {
	case ModeOff, ModePrivate, ModePublic:
		return m, true
	}
	return "", false
}

// Defaults for the variables above.
const (
	DefaultKeep        = 3
	DefaultIdleMinutes = 5
	DefaultInterval    = time.Minute
	DefaultMaxCopies   = 1
	DefaultMinFreeMB   = 1024
	DefaultMaxMemPct   = 25
	keyLen             = 7
)

var previewName = regexp.MustCompile(`-pr-\d+$`)

// Config is the environment-driven tuning.
type Config struct {
	Keep        int
	IdleMinutes int
	Interval    time.Duration
	MaxCopies   int
	MinFreeMB   int
	MaxMemPct   int
	// Mode is the env-seeded copies mode; a stored setting wins over it (see Syncer.Mode).
	Mode string
}

// ConfigFromEnv reads the Env* variables, using defaults for anything missing or malformed.
func ConfigFromEnv(getenv func(string) string) Config {
	cfg := Config{Keep: DefaultKeep, IdleMinutes: DefaultIdleMinutes, Interval: DefaultInterval,
		MaxCopies: DefaultMaxCopies, MinFreeMB: DefaultMinFreeMB, MaxMemPct: DefaultMaxMemPct, Mode: DefaultMode}
	if m, ok := ParseMode(getenv(EnvCopies)); ok {
		cfg.Mode = m
	}
	if n, err := strconv.Atoi(strings.TrimSpace(getenv(EnvMaxCopies))); err == nil && n >= 0 {
		cfg.MaxCopies = n
	}
	if n, err := strconv.Atoi(strings.TrimSpace(getenv(EnvMinFreeMemoryMB))); err == nil && n >= 0 {
		cfg.MinFreeMB = n
	}
	if n, err := strconv.Atoi(strings.TrimSpace(getenv(EnvMaxMemoryPercent))); err == nil && n >= 0 && n <= 100 {
		cfg.MaxMemPct = n
	}
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
// It carries the host only in public mode (host is "" otherwise), so a private
// copy has no route and no certificate order.
func Clone(app store.DesiredService, w Wanted, host string) store.DesiredService {
	svc, ref, _ := deploy.RollbackDesired(app, w.Attempt, w.Pin)
	svc.Name = w.ServiceName
	svc.Image = ref
	svc.ImageID, svc.ImageIDRef = "", ""
	svc.Domains = domainsFor(host)
	svc.Replicas = 1
	svc.Volumes, svc.BindMounts = nil, nil
	svc.HostPort, svc.PublishedPorts = nil, nil
	svc.Hooks = nil
	svc.Suspended = false
	svc.AutoRollbackOnCrashloop, svc.AutoRollbackOnSLOBurn = false, ""
	svc.ExecEnabled = false
	return svc
}

func domainsFor(host string) []string {
	if host == "" {
		return nil
	}
	return []string{host}
}

// Store is what the syncer needs; *store.DB satisfies it.
type Store interface {
	ListDesiredServices(ctx context.Context) ([]store.DesiredService, error)
	SaveDesiredService(ctx context.Context, svc store.DesiredService) error
	SaveSuspendedDesiredService(ctx context.Context, svc store.DesiredService) error
	ListAppSleep(ctx context.Context) ([]store.AppSleep, error)
	DeleteAppSleep(ctx context.Context, serviceName string) error
	// DeleteAppSleepIfOrphan deletes only while no desired service has the name.
	DeleteAppSleepIfOrphan(ctx context.Context, serviceName string) error
	ListDeployAttempts(ctx context.Context, serviceName string) ([]store.DeployAttempt, error)
	ListRollbackPins(ctx context.Context, serviceName string) ([]store.RollbackPin, error)
	SetAppSleepIdle(ctx context.Context, serviceName string, minutes int, now time.Time) error
	SetAppSleepHold(ctx context.Context, serviceName string, hold bool, now time.Time) error
	SetAppSleeping(ctx context.Context, serviceName string, sleeping bool, now time.Time) error
	ListDeployURLs(ctx context.Context, appName string) ([]store.DeployURL, error)
	SaveDeployURL(ctx context.Context, u store.DeployURL) error
	DeleteDeployURL(ctx context.Context, serviceName string) error
}

// Syncer reconciles retained deploy services against each app's recent deploys.
type Syncer struct {
	Store  Store
	Config Config
	// Mode returns the stored copies mode, or "" to fall back to Config.Mode.
	Mode func(ctx context.Context) string
	// HostFor returns the hostname for a retained service, or "" when none can be formed.
	HostFor func(ctx context.Context, serviceName string) string
	// Remove deletes a retained service and tears its containers down.
	Remove func(ctx context.Context, serviceName string) error
	// Nudge asks the reconcilers to run now.
	Nudge func()
	// HostMemory reports host total and available bytes; nil reads this host.
	HostMemory func() (total, avail int64, err error)
	Now        func() time.Time
	Logger     *slog.Logger

	existing map[string]bool
	byName   map[string]store.DesiredService
	optOut   map[string]bool
	mode     string
}

// effectiveMode is the stored mode when valid, else the env seed, else the default.
func (s *Syncer) effectiveMode(ctx context.Context) string {
	if s.Mode != nil {
		if m, ok := ParseMode(s.Mode(ctx)); ok {
			return m
		}
	}
	if m, ok := ParseMode(s.Config.Mode); ok {
		return m
	}
	return DefaultMode
}

func (s *Syncer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Sync runs one pass. A failure on one app is logged and does not stop the rest.
func (s *Syncer) Sync(ctx context.Context) error {
	// Sleep rows are read before services: a service created in between
	// then still counts as existing and its fresh row is never pruned.
	sleeps, err := s.Store.ListAppSleep(ctx)
	if err != nil {
		return fmt.Errorf("deployurls: list sleep rows: %w", err)
	}
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
	s.byName = make(map[string]store.DesiredService, len(services))
	for _, svc := range services {
		s.byName[svc.Name] = svc
	}
	s.mode = s.effectiveMode(ctx)
	s.optOut = s.loadOptOuts(ctx)
	s.pruneOrphanSleep(ctx, sleeps, existing)
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

// pruneOrphanSleep clears sleep settings whose service is gone, which older
// app deletes left behind and `apps sleep list` showed as awake.
func (s *Syncer) pruneOrphanSleep(ctx context.Context, sleeps []store.AppSleep, existing map[string]bool) {
	for _, a := range sleeps {
		if existing[a.ServiceName] {
			continue
		}
		if err := s.Store.DeleteAppSleepIfOrphan(ctx, a.ServiceName); err != nil {
			s.Logger.WarnContext(ctx, "deploy urls: prune orphan sleep row failed", slog.String("service", a.ServiceName), slog.String("error", err.Error()))
		}
	}
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
	if s.Config.Keep > 0 && s.mode != ModeOff && !app.Suspended && ExcludeReason(app) == "" {
		attempts, err := s.Store.ListDeployAttempts(ctx, app.Name)
		if err != nil {
			return false, fmt.Errorf("list attempts: %w", err)
		}
		pins, err := s.Store.ListRollbackPins(ctx, app.Name)
		if err != nil {
			return false, fmt.Errorf("list pins: %w", err)
		}
		wanted = capCopies(Plan(app, attempts, pins, s.Config.Keep), s.Config.MaxCopies)
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
		if s.memoryTight(app) {
			s.Logger.InfoContext(ctx, "deploy urls: copy skipped, host memory is tight", slog.String("app", app.Name))
			continue
		}
		host := s.hostFor(ctx, app.Name, w.ServiceName)
		if s.mode == ModePublic && host == "" {
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
			continue
		}
		if s.realign(ctx, row) {
			changed = true
		}
	}
	return changed, nil
}

// loadOptOuts reads the apps that opted out of automatic hostnames; a store
// without the table or a failed read means nobody opted out.
func (s *Syncer) loadOptOuts(ctx context.Context) map[string]bool {
	src, ok := s.Store.(interface {
		ListAppAutoHostOptOuts(ctx context.Context) (map[string]bool, error)
	})
	if !ok {
		return nil
	}
	m, err := src.ListAppAutoHostOptOuts(ctx)
	if err != nil {
		return nil
	}
	return m
}

// hostFor is empty for an opted-out app: its copies must not order a certificate either.
func (s *Syncer) hostFor(ctx context.Context, app, serviceName string) string {
	if s.mode != ModePublic || s.HostFor == nil || s.optOut[app] {
		return ""
	}
	return s.HostFor(ctx, serviceName)
}

// realign moves an existing copy to the current mode by adding or dropping its
// hostname; the sleep state and suspended flag are left as they are.
func (s *Syncer) realign(ctx context.Context, row store.DeployURL) bool {
	svc, ok := s.byName[row.ServiceName]
	if !ok {
		return false
	}
	host := s.hostFor(ctx, row.AppName, row.ServiceName)
	// An opted-out app's copy drops its host; otherwise an empty host is a transient miss.
	if s.mode == ModePublic && host == "" && !s.optOut[row.AppName] {
		return false
	}
	if row.Host == host {
		return false
	}
	svc.Domains = domainsFor(host)
	if err := s.Store.SaveDesiredService(ctx, svc); err != nil {
		s.Logger.WarnContext(ctx, "deploy urls: realign failed", slog.String("service", row.ServiceName), slog.String("error", err.Error()))
		return false
	}
	row.Host = host
	if err := s.Store.SaveDeployURL(ctx, row); err != nil {
		s.Logger.WarnContext(ctx, "deploy urls: realign row failed", slog.String("service", row.ServiceName), slog.String("error", err.Error()))
		return false
	}
	return true
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
	// Born asleep: an old deploy costs nothing until someone opens its URL.
	svc := Clone(app, w, host)
	// One write, so a reconcile landing before the stop cannot start a container.
	if err := s.Store.SaveSuspendedDesiredService(ctx, svc); err != nil {
		return fmt.Errorf("save service: %w", err)
	}
	now := s.now()
	if err := s.Store.SetAppSleepIdle(ctx, w.ServiceName, s.Config.IdleMinutes, now); err != nil {
		return fmt.Errorf("enable sleep: %w", err)
	}
	if err := s.Store.SetAppSleepHold(ctx, w.ServiceName, true, now); err != nil {
		return fmt.Errorf("hold requests: %w", err)
	}
	if err := s.Store.SetAppSleeping(ctx, w.ServiceName, true, now); err != nil {
		return fmt.Errorf("mark asleep: %w", err)
	}
	return s.Store.SaveDeployURL(ctx, store.DeployURL{ServiceName: w.ServiceName, AppName: app.Name, AttemptID: w.Attempt.ID, Host: host, CreatedAt: now})
}

// Run syncs every Config.Interval until ctx ends.
func (s *Syncer) Run(ctx context.Context) error {
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

// ExcludeReason names why app gets no retained copies, or "" when it may. A
// copy runs without the app's volumes and host ports, so for an app that owns
// data or a host claim it would be an empty second instance, never a rollback target.
func ExcludeReason(app store.DesiredService) string {
	if len(app.Volumes) > 0 {
		return "named volume"
	}
	for _, m := range app.BindMounts {
		if !m.ReadOnly && !strings.HasSuffix(path.Clean(m.HostPath), ".sock") {
			return "writable bind mount"
		}
	}
	if app.HostPort != nil || len(app.PublishedPorts) > 0 {
		return "host port"
	}
	return ""
}

// capCopies keeps the live entry (it needs no service) and at most max older ones.
func capCopies(in []Wanted, limit int) []Wanted {
	out := in[:0:0]
	copies := 0
	for _, w := range in {
		if !w.Live {
			if copies >= limit {
				continue
			}
			copies++
		}
		out = append(out, w)
	}
	return out
}

// memoryTight reports whether creating a copy of app could push the host into
// memory pressure. An unreadable host memory never blocks.
func (s *Syncer) memoryTight(app store.DesiredService) bool {
	read := s.HostMemory
	if read == nil {
		read = telemetry.HostMemoryBytes
	}
	total, avail, err := read()
	if err != nil || total <= 0 {
		return false
	}
	if mb := int64(s.Config.MinFreeMB); mb > 0 && avail < mb<<20 {
		return true
	}
	if pct := int64(s.Config.MaxMemPct); pct > 0 && app.Resources != nil && app.Resources.MemoryBytes*100 > total*pct {
		return true
	}
	return false
}
