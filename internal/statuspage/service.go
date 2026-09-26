package statuspage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/netguard"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Store is what Service reads and writes. *store.DB satisfies it.
type Store interface {
	GetStatusPageSettings(ctx context.Context) (store.StatusPageSettings, error)
	ListStatusComponents(ctx context.Context) ([]store.StatusComponent, error)
	AddStatusSamples(ctx context.Context, componentID, day string, ok, degraded, down int) error
	ListStatusDaily(ctx context.Context, sinceDay string) ([]store.StatusDaily, error)
	PruneStatusDaily(ctx context.Context, beforeDay string) error
	ListStatusIncidents(ctx context.Context, limit int) ([]store.StatusIncident, error)
	ListStatusIncidentUpdates(ctx context.Context) ([]store.StatusIncidentUpdate, error)
}

// AppSource reports an app's current status as one of the status constants.
type AppSource interface {
	AppStatus(ctx context.Context, name string) (string, error)
}

// DomainSource is the existing DNS domain check.
type DomainSource interface {
	CheckDomainStatus(ctx context.Context, domain string) (string, error)
}

// Config tunes sampling and caching; zero values take the defaults.
type Config struct {
	SampleInterval time.Duration
	CacheTTL       time.Duration
	CheckTimeout   time.Duration
	Parallelism    int
}

// Defaults for Config, overridable via environment in cmd/levelrail.
const (
	DefaultSampleInterval = time.Minute
	DefaultCacheTTL       = 30 * time.Second
	DefaultCheckTimeout   = 8 * time.Second
	defaultParallelism    = 8
)

// Service samples component health and builds the public View.
type Service struct {
	store   Store
	apps    AppSource
	domains DomainSource
	client  *http.Client
	cfg     Config
	logger  *slog.Logger
	now     func() time.Time

	mu       sync.Mutex
	current  map[string]string
	cached   *View
	cachedAt time.Time
}

// New builds a Service. apps and domains may be nil, in which case
// components of that kind report unknown.
func New(st Store, apps AppSource, domains DomainSource, cfg Config, logger *slog.Logger) *Service {
	if cfg.SampleInterval <= 0 {
		cfg.SampleInterval = DefaultSampleInterval
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = DefaultCacheTTL
	}
	if cfg.CheckTimeout <= 0 {
		cfg.CheckTimeout = DefaultCheckTimeout
	}
	if cfg.Parallelism <= 0 {
		cfg.Parallelism = defaultParallelism
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: st, apps: apps, domains: domains, client: netguard.NewClient(), cfg: cfg, logger: logger,
		now: time.Now, current: map[string]string{}}
}

// Invalidate drops the cached view so the next read reflects a management change.
func (s *Service) Invalidate() {
	s.mu.Lock()
	s.cached = nil
	s.mu.Unlock()
}

// Run samples every component on the configured interval until ctx ends.
func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.SampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.Sample(ctx); err != nil {
				s.logger.Warn("statuspage: sample failed", slog.String("error", err.Error()))
			}
		}
	}
}

// Sample probes every component once and records the result. It does
// nothing while the page is disabled.
func (s *Service) Sample(ctx context.Context) error {
	settings, err := s.store.GetStatusPageSettings(ctx)
	if err != nil {
		return fmt.Errorf("statuspage: load settings: %w", err)
	}
	if !settings.Enabled {
		return nil
	}
	comps, err := s.store.ListStatusComponents(ctx)
	if err != nil {
		return fmt.Errorf("statuspage: list components: %w", err)
	}

	results := make([]string, len(comps))
	sem := make(chan struct{}, s.cfg.Parallelism)
	var wg sync.WaitGroup
	for i, c := range comps {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = s.probe(ctx, c)
		}()
	}
	wg.Wait()

	day := s.now().UTC().Format("2006-01-02")
	current := make(map[string]string, len(comps))
	var errs []error
	for i, c := range comps {
		current[c.ID] = results[i]
		ok, deg, down := 0, 0, 0
		switch results[i] {
		case Operational:
			ok = 1
		case Degraded:
			deg = 1
		case Outage:
			down = 1
		default:
			continue
		}
		if err := s.store.AddStatusSamples(ctx, c.ID, day, ok, deg, down); err != nil {
			errs = append(errs, err)
		}
	}
	s.mu.Lock()
	s.current = current
	s.mu.Unlock()

	cutoff := s.now().UTC().AddDate(0, 0, -UptimeDays).Format("2006-01-02")
	if err := s.store.PruneStatusDaily(ctx, cutoff); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (s *Service) probe(ctx context.Context, c store.StatusComponent) string {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.CheckTimeout)
	defer cancel()
	switch c.Kind {
	case KindApp:
		if s.apps == nil {
			return Unknown
		}
		st, err := s.apps.AppStatus(ctx, c.Target)
		if err != nil {
			return Unknown
		}
		return normalize(st)
	case KindDomain:
		if s.domains == nil {
			return Unknown
		}
		st, err := s.domains.CheckDomainStatus(ctx, c.Target)
		if err != nil {
			return Unknown
		}
		switch st {
		case "connected":
			return Operational
		case "unconfigured":
			return Unknown
		default:
			return Outage
		}
	case KindCheck:
		return s.httpCheck(ctx, c.Target)
	default:
		return Unknown
	}
}

func normalize(st string) string {
	switch st {
	case Operational, Degraded, Outage, Maintenance:
		return st
	default:
		return Unknown
	}
}

func (s *Service) httpCheck(ctx context.Context, target string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Unknown
	}
	resp, err := s.client.Do(req)
	if err != nil {
		if errors.Is(err, netguard.ErrBlockedAddress) {
			return Unknown
		}
		return Outage
	}
	_ = resp.Body.Close()
	switch {
	case resp.StatusCode >= 500:
		return Outage
	case resp.StatusCode >= 400:
		return Degraded
	default:
		return Operational
	}
}

// View returns the public view, cached for the configured TTL.
func (s *Service) View(ctx context.Context) (View, error) {
	s.mu.Lock()
	if s.cached != nil && s.now().Sub(s.cachedAt) < s.cfg.CacheTTL {
		v := *s.cached
		s.mu.Unlock()
		return v, nil
	}
	s.mu.Unlock()

	v, err := s.build(ctx)
	if err != nil {
		return View{}, err
	}
	s.mu.Lock()
	s.cached, s.cachedAt = &v, s.now()
	s.mu.Unlock()
	return v, nil
}

// Enabled reports whether the public page is switched on.
func (s *Service) Enabled(ctx context.Context) (bool, error) {
	settings, err := s.store.GetStatusPageSettings(ctx)
	if err != nil {
		return false, fmt.Errorf("statuspage: load settings: %w", err)
	}
	return settings.Enabled, nil
}

// PreviewView builds an uncached view regardless of the enabled flag, for the management UI.
func (s *Service) PreviewView(ctx context.Context) (View, error) { return s.build(ctx) }

func (s *Service) build(ctx context.Context) (View, error) {
	settings, err := s.store.GetStatusPageSettings(ctx)
	if err != nil {
		return View{}, fmt.Errorf("statuspage: load settings: %w", err)
	}
	comps, err := s.store.ListStatusComponents(ctx)
	if err != nil {
		return View{}, fmt.Errorf("statuspage: list components: %w", err)
	}
	now := s.now().UTC()
	daily, err := s.store.ListStatusDaily(ctx, now.AddDate(0, 0, -UptimeDays).Format("2006-01-02"))
	if err != nil {
		return View{}, fmt.Errorf("statuspage: list daily: %w", err)
	}
	incidents, err := s.store.ListStatusIncidents(ctx, 200)
	if err != nil {
		return View{}, fmt.Errorf("statuspage: list incidents: %w", err)
	}
	updates, err := s.store.ListStatusIncidentUpdates(ctx)
	if err != nil {
		return View{}, fmt.Errorf("statuspage: list incident updates: %w", err)
	}

	s.mu.Lock()
	current := make(map[string]string, len(s.current))
	for k, v := range s.current {
		current[k] = v
	}
	s.mu.Unlock()

	return assemble(settings, comps, daily, incidents, updates, current, now), nil
}

// assemble is the pure core of build: it maps stored data to the
// whitelisted View, dropping every internal identifier and target.
func assemble(settings store.StatusPageSettings, comps []store.StatusComponent, daily []store.StatusDaily,
	incidents []store.StatusIncident, updates []store.StatusIncidentUpdate, current map[string]string, now time.Time) View {
	names := make(map[string]string, len(comps))
	dailyBy := map[string][]store.StatusDaily{}
	for _, c := range comps {
		names[c.ID] = c.DisplayName
	}
	for _, d := range daily {
		dailyBy[d.ComponentID] = append(dailyBy[d.ComponentID], d)
	}
	updatesBy := map[string][]UpdateView{}
	for _, u := range updates {
		updatesBy[u.IncidentID] = append(updatesBy[u.IncidentID], UpdateView{Status: u.Status, Body: u.Body, At: u.CreatedAt.UTC()})
	}

	v := View{Title: settings.Title, Description: settings.Description, GeneratedAt: now,
		Components: []ComponentView{}, Incidents: []IncidentView{}, Maintenance: []IncidentView{}}
	if v.Title == "" {
		v.Title = "Service status"
	}

	impact := map[string]string{}
	inMaintenance := map[string]bool{}
	for _, inc := range incidents {
		view := incidentView(inc, names, updatesBy[inc.ID])
		switch inc.Kind {
		case MaintenanceKind:
			if inc.Status == StatusCompleted && now.Sub(endOrStart(inc)) > recentWindow {
				continue
			}
			v.Maintenance = append(v.Maintenance, view)
			if maintenanceActive(inc, now) {
				for _, id := range inc.ComponentIDs {
					inMaintenance[id] = true
				}
			}
		default:
			if inc.Status == StatusResolved && inc.ResolvedAt != nil && now.Sub(*inc.ResolvedAt) > recentWindow {
				continue
			}
			v.Incidents = append(v.Incidents, view)
			if inc.Status != StatusResolved {
				for _, id := range inc.ComponentIDs {
					impact[id] = maxStatus(impact[id], impactStatus(inc.Impact))
				}
			}
		}
	}

	statuses := make([]string, 0, len(comps))
	for _, c := range comps {
		status := current[c.ID]
		if status == "" {
			status = Unknown
		}
		if o, ok := impact[c.ID]; ok {
			status = combine(status, o)
		}
		if inMaintenance[c.ID] {
			status = Maintenance
		}
		days, up := buildBars(dailyBy[c.ID], now)
		v.Components = append(v.Components, ComponentView{Name: c.DisplayName, Status: status, Uptime90: up, Days: days})
		statuses = append(statuses, status)
	}
	v.Status = OverallStatus(statuses)
	v.StatusText = StatusText(v.Status)
	return v
}

func endOrStart(i store.StatusIncident) time.Time {
	if i.EndsAt != nil {
		return *i.EndsAt
	}
	return i.StartsAt
}

func maintenanceActive(i store.StatusIncident, now time.Time) bool {
	if i.Status == StatusInProgress {
		return true
	}
	return i.Status == StatusScheduled && !now.Before(i.StartsAt) && (i.EndsAt == nil || now.Before(*i.EndsAt))
}

func maxStatus(a, b string) string {
	if severity(b) > severity(a) {
		return b
	}
	return a
}

// combine returns the worse of the probe result and the operator-declared impact.
func combine(probe, override string) string {
	if severity(override) > severity(probe) || probe == Unknown && override != Operational {
		return override
	}
	return probe
}

func incidentView(i store.StatusIncident, names map[string]string, updates []UpdateView) IncidentView {
	comps := make([]string, 0, len(i.ComponentIDs))
	for _, id := range i.ComponentIDs {
		if n, ok := names[id]; ok {
			comps = append(comps, n)
		}
	}
	if updates == nil {
		updates = []UpdateView{}
	}
	return IncidentView{ID: i.ID, Title: i.Title, Status: i.Status, Impact: i.Impact, StartsAt: i.StartsAt.UTC(),
		EndsAt: i.EndsAt, ResolvedAt: i.ResolvedAt, Components: comps, Updates: updates}
}
