// Package stackdrift periodically plans each stack's latest applied revision
// against live state and records whether they still match (ADR 042). It only
// reads: nothing here applies a plan or writes a live resource.
package stackdrift

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Env names for the loop. APP_STACK_DRIFT_INTERVAL=0 turns it off.
const (
	EnvInterval   = "APP_STACK_DRIFT_INTERVAL"
	EnvSummaryMax = "APP_STACK_DRIFT_SUMMARY_MAX"

	DefaultInterval   = 15 * time.Minute
	defaultSummaryMax = 50
	maxPathLen        = 120
)

// Item actions in a drift summary.
const (
	ActionChanged      = "changed"
	ActionMissing      = "missing"
	ActionOwnerChanged = "owner_changed"
)

// IntervalFromEnv reads APP_STACK_DRIFT_INTERVAL; 0 disables the loop and an
// unset or invalid value falls back to the default.
func IntervalFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv(EnvInterval))
	if raw == "" {
		return DefaultInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		return DefaultInterval
	}
	return d
}

func summaryMaxFromEnv() int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(EnvSummaryMax))); err == nil && n > 0 {
		return n
	}
	return defaultSummaryMax
}

// Store is the store surface the checker uses.
type Store interface {
	ListStacks(ctx context.Context) ([]store.StackRecord, error)
	ListStackRevisions(ctx context.Context, stackID string, limit int) ([]store.StackRevisionRecord, error)
	GetStackRevision(ctx context.Context, stackID string, number int) (store.StackRevisionRecord, error)
	ListStackLedger(ctx context.Context, stackID string) ([]store.StackLedgerEntry, error)
	GetStackResourceOwner(ctx context.Context, kind, key string) (store.StackLedgerEntry, bool, error)
	SetStackDrift(ctx context.Context, d store.StackDrift) error
}

// OpenLive returns a read-only client for live state and a release func the
// checker calls when the pass is done.
type OpenLive func(ctx context.Context) (iac.Doer, func(), error)

// Checker records drift for every stack.
type Checker struct {
	Store Store
	Live  OpenLive
	// ProjectName resolves a stack's project id to the name documents use.
	ProjectName func(ctx context.Context, id string) string
	Logger      *slog.Logger
	Now         func() time.Time
	// StartDelay is the wait before the first pass after boot.
	StartDelay time.Duration
}

func (c *Checker) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Checker) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// Run checks every interval until ctx ends. Interval <= 0 returns at once.
func (c *Checker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		c.logger().Info("stack drift checks disabled (" + EnvInterval + "=0)")
		return
	}
	wait := c.StartDelay
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		c.CheckAll(ctx)
		wait = interval
	}
}

// CheckAll runs one pass. A failure on one stack is logged and never stops
// the others or the loop.
func (c *Checker) CheckAll(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.logger().Error("stack drift: pass panicked", slog.Any("panic", r))
		}
	}()
	stacks, err := c.Store.ListStacks(ctx)
	if err != nil {
		c.logger().Warn("stack drift: list stacks failed", slog.String("error", err.Error()))
		return
	}
	names := make(map[string]string, len(stacks))
	for _, s := range stacks {
		names[s.ID] = s.Name
	}
	live, release, err := c.Live(ctx)
	if err != nil {
		c.logger().Warn("stack drift: open live access failed", slog.String("error", err.Error()))
		return
	}
	defer release()
	for _, s := range stacks {
		if ctx.Err() != nil {
			return
		}
		if err := c.checkSafely(ctx, live, s, names); err != nil {
			c.logger().Warn("stack drift: check failed", slog.String("stack", s.Name), slog.String("error", err.Error()))
		}
	}
}

func (c *Checker) checkSafely(ctx context.Context, live iac.Doer, s store.StackRecord, names map[string]string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return c.checkStack(ctx, live, s, names)
}

type renderedFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func (c *Checker) checkStack(ctx context.Context, live iac.Doer, s store.StackRecord, names map[string]string) error {
	revs, err := c.Store.ListStackRevisions(ctx, s.ID, 0)
	if err != nil {
		return err
	}
	number := 0
	for _, v := range revs {
		if v.Status == store.StackRevApplied {
			number = v.Number
			break
		}
	}
	if number == 0 {
		return nil
	}
	rev, err := c.Store.GetStackRevision(ctx, s.ID, number)
	if err != nil {
		return err
	}
	project := ""
	if c.ProjectName != nil {
		project = c.ProjectName(ctx, s.ProjectID)
	}
	res, opts, err := build(rev.RenderedDocs, s.Name, project)
	if err != nil {
		return err
	}
	lctx, err := c.ledgerContext(ctx, s, project, res, names)
	if err != nil {
		return err
	}
	st, err := iac.Load(lctx, live, res, opts)
	if err != nil {
		return fmt.Errorf("read live state: %w", err)
	}
	plan := iac.PlanFor(st, res, opts)
	items := Summarize(plan, summaryMaxFromEnv())
	status := store.StackDriftNone
	if len(items) > 0 {
		status = store.StackDriftDrifted
	}
	return c.Store.SetStackDrift(ctx, store.StackDrift{StackID: s.ID, Status: status, Revision: number, Summary: items, CheckedAt: c.now().UTC()})
}

// build turns a revision's rendered documents into resources, the same way the
// stack plan endpoint does for its files.
func build(raw []byte, stack, project string) ([]*iac.Resource, iac.Options, error) {
	var files []renderedFile
	if err := json.Unmarshal(raw, &files); err != nil {
		return nil, iac.Options{}, fmt.Errorf("decode rendered documents: %w", err)
	}
	srcs := make([]iac.Source, len(files))
	for i, f := range files {
		srcs[i] = iac.Source{Name: f.Name, Data: []byte(f.Content)}
	}
	opts := iac.Options{Source: stack, Project: project}
	docs, issues := iac.ParseDocuments(srcs)
	if len(issues) == 0 {
		if project != "" {
			docs = iac.FilterProject(docs, project)
		}
		var res []*iac.Resource
		if res, issues = iac.Build(docs, opts); len(issues) == 0 {
			return res, opts, nil
		}
	}
	return nil, opts, errors.New("revision documents no longer validate: " + issues[0].Message)
}

// ledgerContext mirrors the stack plan endpoint: the stack's own rows plus the
// owner of every declared key another stack holds. Lookups fail closed.
func (c *Checker) ledgerContext(ctx context.Context, s store.StackRecord, project string, res []*iac.Resource, names map[string]string) (context.Context, error) {
	rows, err := c.Store.ListStackLedger(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	owned := make([]iac.LedgerEntry, 0, len(rows))
	for _, e := range rows {
		owned = append(owned, iac.LedgerEntry{StackID: e.StackID, Kind: iac.Kind(e.Kind), Key: e.Key, LiveID: e.LiveID, SinceRevision: e.SinceRevision})
	}
	l := iac.NewLedger(s.ID, s.Name, project, owned)
	for _, ref := range iac.LedgerRefs(res) {
		owner, found, err := c.Store.GetStackResourceOwner(ctx, string(ref.Kind), ref.Key)
		if err != nil {
			return nil, err
		}
		if found && owner.StackID != s.ID {
			if n, ok := names[owner.StackID]; ok {
				l.Foreign[ref.Key] = n
			} else {
				l.Foreign[ref.Key] = owner.StackID
			}
		}
	}
	return iac.WithLedger(ctx, l), nil
}

// Summarize reduces a plan to drift items: field paths only, never the old or
// new values, so env, labels and commands stay hidden. At most max items.
func Summarize(p iac.Plan, limit int) []store.StackDriftItem {
	foreign := map[string]bool{}
	for _, k := range iac.LedgerConflicts(p) {
		foreign[k] = true
	}
	items := []store.StackDriftItem{}
	for _, ch := range p.Changes {
		it := store.StackDriftItem{Kind: string(ch.Kind), Name: ch.Name, Scope: ch.Scope}
		switch {
		case ch.Action == iac.ActionUpdate:
			it.Action = ActionChanged
			for _, f := range ch.Fields {
				path := f.Path
				if len(path) > maxPathLen {
					path = path[:maxPathLen]
				}
				it.Fields = append(it.Fields, path)
			}
		case ch.Action == iac.ActionCreate:
			it.Action = ActionMissing
		case ch.Action == iac.ActionError && foreign[ch.Key()]:
			it.Action = ActionOwnerChanged
		default:
			continue
		}
		if len(items) == limit {
			break
		}
		items = append(items, it)
	}
	return items
}
