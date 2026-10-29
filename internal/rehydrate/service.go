package rehydrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// RunStore persists the latest report so it survives a restart mid-recovery.
type RunStore interface {
	SaveRehydrateRun(ctx context.Context, r store.RehydrateRun) error
	LatestRehydrateRun(ctx context.Context) (store.RehydrateRun, error)
}

// Service plans, runs and remembers rehydrate passes. One run at a time.
type Service struct {
	Inventory Inventory
	Restorer  Restorer // nil means restores are not configured
	Store     RunStore // nil keeps the report in memory only
	Config    Config
	Logger    *slog.Logger
	Now       func() time.Time

	mu      sync.Mutex
	running bool
	current *Report
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Plan is a dry run: it changes nothing.
func (s *Service) Plan(ctx context.Context, opts PlanOptions) (Report, error) {
	return BuildPlan(ctx, s.Inventory, s.Config, opts, s.now())
}

// Start builds the plan and works it in the background. It returns the report
// in its initial running state.
func (s *Service) Start(ctx context.Context, opts PlanOptions) (Report, error) {
	if s.Restorer == nil {
		return Report{}, ErrNotConfigured
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return Report{}, ErrBusy
	}
	s.running = true
	s.mu.Unlock()

	rep, err := BuildPlan(ctx, s.Inventory, s.Config, opts, s.now())
	if err != nil {
		s.release()
		return Report{}, err
	}
	rep.Mode, rep.State = ModeRun, StateRunning
	s.mu.Lock()
	s.current = &rep
	snap := snapshot(&rep)
	s.mu.Unlock()
	s.persist(ctx, snap)
	go s.execute(&rep) //nolint:gosec // the run outlives the request that started it
	return snap, nil
}

func (s *Service) release() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

func snapshot(r *Report) Report {
	cp := *r
	cp.Items = append([]Item(nil), r.Items...)
	return cp
}

// execute works the plan with bounded concurrency. A background context is
// deliberate: the request that started the run returns long before it ends.
func (s *Service) execute(rep *Report) {
	defer s.release()
	ctx := context.Background()
	conc := s.Config.Concurrency
	if conc < 1 {
		conc = 1
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for i := range rep.Items {
		if rep.Items[i].Status != StatusPlanned {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			s.restoreItem(ctx, rep, i)
		}(i)
	}
	wg.Wait()

	s.mu.Lock()
	fin := s.now().UTC()
	rep.FinishedAt = &fin
	rep.Summary = summarize(rep.Items)
	rep.State = finalState(rep.Summary)
	final := snapshot(rep)
	s.mu.Unlock()
	s.persist(ctx, final)
}

func (s *Service) restoreItem(ctx context.Context, rep *Report, i int) {
	s.mu.Lock()
	started := s.now().UTC()
	rep.Items[i].Status, rep.Items[i].StartedAt = StatusRunning, &started
	it := rep.Items[i]
	s.mu.Unlock()

	var (
		id  string
		err error
	)
	if it.Kind == KindVolume {
		id, err = s.Restorer.RestoreVolume(ctx, Volume{App: it.App, Name: it.Volume}, it.Backup.ID)
	} else {
		id, err = s.Restorer.RestoreDatabase(ctx, Database{Name: it.Database, Engine: it.Engine}, it.Backup.ID)
	}

	s.mu.Lock()
	done := s.now().UTC()
	rep.Items[i].RestoreID, rep.Items[i].FinishedAt = id, &done
	if err != nil {
		rep.Items[i].Status, rep.Items[i].Error = StatusFailed, err.Error()
	} else {
		rep.Items[i].Status = StatusSucceeded
	}
	rep.Summary = summarize(rep.Items)
	s.mu.Unlock()
	if err != nil && s.Logger != nil {
		s.Logger.Error("rehydrate: restore failed", slog.String("item", it.ID), slog.String("error", err.Error()))
	}
}

func finalState(sum Summary) string {
	switch {
	case sum.Failed == 0 && sum.Blocked == 0:
		return StateSucceeded
	case sum.Succeeded == 0:
		return StateFailed
	default:
		return StatePartial
	}
}

// Report returns the live run when one exists, else the last saved report.
func (s *Service) Report(ctx context.Context) (Report, bool, error) {
	s.mu.Lock()
	if s.current != nil {
		snap := snapshot(s.current)
		s.mu.Unlock()
		return snap, true, nil
	}
	s.mu.Unlock()
	if s.Store == nil {
		return Report{}, false, nil
	}
	row, err := s.Store.LatestRehydrateRun(ctx)
	if errors.Is(err, store.ErrRehydrateRunNotFound) {
		return Report{}, false, nil
	}
	if err != nil {
		return Report{}, false, err
	}
	var rep Report
	if err := json.Unmarshal([]byte(row.Report), &rep); err != nil {
		return Report{}, false, fmt.Errorf("rehydrate: decode saved report: %w", err)
	}
	return rep, true, nil
}

func (s *Service) persist(ctx context.Context, rep Report) {
	if s.Store == nil {
		return
	}
	raw, err := json.Marshal(rep)
	if err != nil {
		return
	}
	row := store.RehydrateRun{ID: rep.ID, State: rep.State, StartedAt: rep.GeneratedAt.Format(time.RFC3339), Report: string(raw)}
	if rep.FinishedAt != nil {
		row.FinishedAt = rep.FinishedAt.Format(time.RFC3339)
	}
	if err := s.Store.SaveRehydrateRun(ctx, row); err != nil && s.Logger != nil {
		s.Logger.Warn("rehydrate: save report failed", slog.String("error", err.Error()))
	}
}
