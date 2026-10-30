package monitor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Store is the persistence the engine needs.
type Store interface {
	ResultSink
	ListMonitors(ctx context.Context) ([]Monitor, error)
	GetMonitor(ctx context.Context, id string) (Monitor, error)
	CountMonitors(ctx context.Context) (int, error)
	SaveMonitor(ctx context.Context, m Monitor) error
	DeleteMonitor(ctx context.Context, id string) error
	ListResults(ctx context.Context, id string, since int64, limit int) ([]Result, error)
	ListRollups(ctx context.Context, id string, sinceBucket int64, limit int) ([]Rollup, error)
	PruneResults(ctx context.Context, id string, before int64, limit int) (int, error)
	PruneRollups(ctx context.Context, id string, before int64, limit int) (int, error)
}

// Secrets keeps header values envelope-encrypted.
type Secrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// Service owns the scheduler, workers, batcher and pruner.
type Service struct {
	cfg     Config
	store   Store
	secrets Secrets
	checker *Checker
	sched   *Scheduler
	batcher *Batcher
	tracker *Tracker
	logger  *slog.Logger
	now     func() time.Time
	onDel   func(id string)
}

// Options configures a Service. Secrets may be nil, which rejects monitors that carry headers.
type Options struct {
	Config  Config
	Store   Store
	Secrets Secrets
	Logger  *slog.Logger
	// Notify receives confirmed down and recovered transitions; nil drops them.
	Notify Notifier
	// OnDelete runs after a monitor is removed so alert state tied to it can clear.
	OnDelete func(id string)
}

// New builds a Service.
func New(o Options) *Service {
	logger := o.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{onDel: o.OnDelete, cfg: o.Config, store: o.Store, secrets: o.Secrets, logger: logger, now: time.Now,
		checker: NewChecker(o.Config), sched: NewScheduler(o.Config, nil), batcher: NewBatcher(o.Store, o.Config.BufferMax),
		tracker: NewTracker(o.Config.ConfirmFailures, o.Config.RecoveryOKs, o.Notify)}
}

// State returns the confirmed state of a monitor, or "" before its first probe.
func (s *Service) State(id string) string { return s.tracker.State(id) }

// Status returns the confirmed state with its consecutive failure and success counts.
func (s *Service) Status(id string) Status { return s.tracker.Status(id) }

// Config returns the engine settings.
func (s *Service) Config() Config { return s.cfg }

func secretService(id string) string { return "monitor-" + id }
func headerKey(name string) string   { return "hdr:" + name }

// Run schedules every stored monitor and blocks until ctx ends.
func (s *Service) Run(ctx context.Context) {
	list, err := s.store.ListMonitors(ctx)
	if err != nil {
		s.logger.Error("monitors: load failed", slog.String("error", err.Error()))
	}
	for _, m := range list {
		s.sched.Add(m, s.now())
	}
	jobs := make(chan Monitor, s.cfg.Workers*2)
	var wg sync.WaitGroup
	for i := 0; i < s.cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for m := range jobs {
				s.probe(ctx, m)
			}
		}()
	}
	var bg sync.WaitGroup
	bg.Add(2)
	go func() { defer bg.Done(); s.flushLoop(ctx) }()
	go func() { defer bg.Done(); s.pruneLoop(ctx) }()

	tick := time.NewTicker(s.cfg.TickInterval)
	defer tick.Stop()
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-tick.C:
			for _, m := range s.sched.Due(s.now()) {
				select {
				case jobs <- m:
				default:
					s.sched.Done(m.ID)
				}
			}
		}
	}
	close(jobs)
	wg.Wait()
	bg.Wait()
	flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.cfg.FlushInterval)
	defer cancel()
	if err := s.batcher.Flush(flushCtx); err != nil {
		s.logger.Warn("monitors: final flush failed", slog.String("error", err.Error()))
	}
}

func (s *Service) probe(ctx context.Context, m Monitor) {
	defer s.sched.Done(m.ID)
	headers, err := s.resolveHeaders(ctx, m)
	if err != nil {
		r := Result{MonitorID: m.ID, At: s.now().Unix(), Reason: ReasonSecretMissing}
		s.batcher.Add(r)
		s.tracker.Observe(m, r)
		return
	}
	r := s.checker.Check(ctx, m, headers)
	s.batcher.Add(r)
	s.tracker.Observe(m, r)
}

func (s *Service) resolveHeaders(ctx context.Context, m Monitor) (map[string]string, error) {
	if len(m.Spec.HeaderNames) == 0 {
		return nil, nil
	}
	if s.secrets == nil {
		return nil, errors.New("secrets unavailable")
	}
	out := make(map[string]string, len(m.Spec.HeaderNames))
	for _, n := range m.Spec.HeaderNames {
		v, err := s.secrets.Resolve(ctx, secretService(m.ID), headerKey(n))
		if err != nil {
			return nil, err
		}
		out[n] = v
	}
	return out, nil
}

func (s *Service) flushLoop(ctx context.Context) {
	t := time.NewTicker(s.cfg.FlushInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.batcher.Flush(ctx); err != nil && ctx.Err() == nil {
				s.logger.Warn("monitors: flush failed", slog.String("error", err.Error()), slog.Uint64("dropped", s.batcher.Dropped()))
			}
		}
	}
}

func (s *Service) pruneLoop(ctx context.Context) {
	t := time.NewTicker(s.cfg.PruneInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.Prune(ctx); err != nil && ctx.Err() == nil {
				s.logger.Warn("monitors: prune failed", slog.String("error", err.Error()))
			}
		}
	}
}

// Prune deletes expired raw results and hourly rollups in bounded batches.
func (s *Service) Prune(ctx context.Context) error {
	list, err := s.store.ListMonitors(ctx)
	if err != nil {
		return fmt.Errorf("monitors: list for prune: %w", err)
	}
	now := s.now()
	rawBefore := now.Add(-s.cfg.RawRetention).Unix()
	hourBefore := HourBucket(now.Add(-s.cfg.HourRetention).Unix())
	for _, m := range list {
		for {
			n, err := s.store.PruneResults(ctx, m.ID, rawBefore, s.cfg.PruneBatch)
			if err != nil {
				return err
			}
			if n < s.cfg.PruneBatch {
				break
			}
		}
		for {
			n, err := s.store.PruneRollups(ctx, m.ID, hourBefore, s.cfg.PruneBatch)
			if err != nil {
				return err
			}
			if n < s.cfg.PruneBatch {
				break
			}
		}
	}
	return nil
}

// CreateInput is a monitor definition plus its header values, which are stored encrypted and never returned.
type CreateInput struct {
	Monitor Monitor
	Headers map[string]string
}

// Create validates, stores and schedules a monitor.
func (s *Service) Create(ctx context.Context, in CreateInput) (Monitor, error) {
	m := in.Monitor
	m.Spec.HeaderNames = nil
	headers := map[string]string{}
	for k, v := range in.Headers {
		if len(v) > s.cfg.HeaderMaxBytes {
			return Monitor{}, fmt.Errorf("%w: header value too long (APP_MONITOR_HEADER_MAX_BYTES)", ErrInvalid)
		}
		name := http.CanonicalHeaderKey(k)
		headers[name] = v
		m.Spec.HeaderNames = append(m.Spec.HeaderNames, name)
	}
	sort.Strings(m.Spec.HeaderNames)
	m = Normalize(m)
	if err := Validate(m, s.cfg); err != nil {
		return Monitor{}, err
	}
	if err := ValidateTarget(ctx, m); err != nil {
		return Monitor{}, err
	}
	if len(headers) > 0 && s.secrets == nil {
		return Monitor{}, fmt.Errorf("%w: the secrets store is not available, headers cannot be stored", ErrInvalid)
	}
	n, err := s.store.CountMonitors(ctx)
	if err != nil {
		return Monitor{}, err
	}
	if n >= s.cfg.MaxCount {
		return Monitor{}, fmt.Errorf("%w: at most %d monitors (APP_MONITOR_MAX_COUNT)", ErrLimit, s.cfg.MaxCount)
	}
	id, err := newID()
	if err != nil {
		return Monitor{}, err
	}
	m.ID, m.CreatedAt = id, s.now().UTC()
	for name, v := range headers {
		if err := s.secrets.SetValue(ctx, secretService(id), headerKey(name), v); err != nil {
			_ = s.secrets.DeleteAll(ctx, secretService(id))
			return Monitor{}, fmt.Errorf("monitors: store header: %w", err)
		}
	}
	if err := s.store.SaveMonitor(ctx, m); err != nil {
		if s.secrets != nil {
			_ = s.secrets.DeleteAll(ctx, secretService(id))
		}
		return Monitor{}, err
	}
	s.sched.Add(m, s.now())
	return m, nil
}

func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("monitors: mint id: %w", err)
	}
	return "mon_" + hex.EncodeToString(b), nil
}

// List returns every monitor.
func (s *Service) List(ctx context.Context) ([]Monitor, error) { return s.store.ListMonitors(ctx) }

// Get returns one monitor or ErrNotFound.
func (s *Service) Get(ctx context.Context, id string) (Monitor, error) {
	return s.store.GetMonitor(ctx, id)
}

// Delete removes a monitor, its history and its stored header values.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.store.DeleteMonitor(ctx, id); err != nil {
		return err
	}
	s.sched.Remove(id)
	s.tracker.Forget(id)
	if s.onDel != nil {
		s.onDel(id)
	}
	if s.secrets != nil {
		if err := s.secrets.DeleteAll(ctx, secretService(id)); err != nil {
			s.logger.Warn("monitors: delete header secrets failed", slog.String("monitor", id), slog.String("error", err.Error()))
		}
	}
	return nil
}

// History is the recent raw results plus hourly rollups for a window.
type History struct {
	Raw    []Result
	Rollup []Rollup
}

// Results returns history for the last window, clamped to the retention periods and the limit cap.
func (s *Service) Results(ctx context.Context, id string, window time.Duration, limit int) (History, error) {
	if _, err := s.store.GetMonitor(ctx, id); err != nil {
		return History{}, err
	}
	if limit <= 0 || limit > s.cfg.ResultsMaxLimit {
		limit = s.cfg.ResultsMaxLimit
	}
	if window <= 0 || window > s.cfg.HourRetention {
		window = s.cfg.HourRetention
	}
	now := s.now()
	rawSince := now.Add(-min(window, s.cfg.RawRetention)).Unix()
	raw, err := s.store.ListResults(ctx, id, rawSince, limit)
	if err != nil {
		return History{}, err
	}
	rows := int(window/time.Hour) + 2
	roll, err := s.store.ListRollups(ctx, id, HourBucket(now.Add(-window).Unix()), rows)
	if err != nil {
		return History{}, err
	}
	return History{Raw: raw, Rollup: roll}, nil
}
