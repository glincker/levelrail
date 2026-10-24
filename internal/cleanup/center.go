package cleanup

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	failureNoticeWindow = 24 * time.Hour
	maxErrorChars       = 200
	recordTimeout       = 10 * time.Second
	auditActorSystem    = "system"
	auditAbilityRoot    = "root"
)

// RunStore persists run history. *store.DB satisfies it.
type RunStore interface {
	InsertCleanupRun(ctx context.Context, r store.CleanupRun) (int64, error)
	ListCleanupRuns(ctx context.Context, f store.CleanupRunFilter) ([]store.CleanupRun, error)
	LatestCleanupRuns(ctx context.Context) ([]store.CleanupRun, error)
	CleanupFailedSince(ctx context.Context, target string, since time.Time) (bool, error)
	PruneCleanupRuns(ctx context.Context, before time.Time) (int64, error)
}

// AuditSaver writes audit rows. *store.DB satisfies it.
type AuditSaver interface {
	SaveAuditEntry(ctx context.Context, e store.AuditEntry) error
}

// Settings resolves runtime settings. *runtimesettings.Service satisfies it, and a nil one yields env then defaults.
type Settings interface {
	Int(ctx context.Context, key string) int
	String(ctx context.Context, key string) string
	Bool(ctx context.Context, key string) bool
	Duration(ctx context.Context, key string) time.Duration
}

// Config wires a Center.
type Config struct {
	Registry *Registry
	Store    RunStore
	Settings Settings
	Audit    AuditSaver
	// Emit hands an event to the event webhook queue; nil drops events.
	Emit func(eventType string, data map[string]any)
	// Notify sends a plain-text notice to the alert channels; nil skips notices.
	Notify func(ctx context.Context, text string) error
	Limits Limits
	Logger *slog.Logger
	Now    func() time.Time
}

// Center runs targets, keeps history and drives the scheduler.
type Center struct {
	cfg Config

	mu      sync.Mutex
	running map[string]bool
	nextRun map[string]time.Time
	started bool
	usage   map[string]usageEntry
}

type usageEntry struct {
	u   Usage
	err string
	at  time.Time
}

// New builds a Center, filling the defaults a caller left out.
func New(cfg Config) *Center {
	if cfg.Registry == nil {
		cfg.Registry = Default
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Limits.MaxRows == 0 {
		cfg.Limits = LimitsFromEnv(func(string) (string, bool) { return "", false })
	}
	return &Center{cfg: cfg, running: map[string]bool{}, nextRun: map[string]time.Time{}, usage: map[string]usageEntry{}}
}

// RunRecord is one history row on the wire.
type RunRecord struct {
	ID         int64     `json:"id"`
	Target     string    `json:"target"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Days       int       `json:"days"`
	DryRun     bool      `json:"dry_run"`
	Rows       int64     `json:"rows"`
	Bytes      int64     `json:"bytes"`
	Trigger    string    `json:"trigger"`
	Actor      string    `json:"actor"`
	Error      string    `json:"error"`
}

func recordOf(r store.CleanupRun) RunRecord {
	return RunRecord{ID: r.ID, Target: r.Target, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, Days: r.Days, DryRun: r.DryRun,
		Rows: r.Rows, Bytes: r.Bytes, Trigger: r.Trigger, Actor: r.Actor, Error: r.Error}
}

// TargetView is one target on the wire.
type TargetView struct {
	ID             string     `json:"id"`
	Category       string     `json:"category"`
	Title          string     `json:"title"`
	TitleKey       string     `json:"title_key,omitempty"`
	Description    string     `json:"description"`
	DescriptionKey string     `json:"description_key,omitempty"`
	PolicyKey      string     `json:"policy_key"`
	Days           int        `json:"days"`
	Mode           string     `json:"mode"`
	Origin         string     `json:"origin"`
	Destructive    bool       `json:"destructive"`
	Floor          int        `json:"floor"`
	Presets        []int      `json:"presets"`
	Usage          Usage      `json:"usage"`
	UsageError     string     `json:"usage_error,omitempty"`
	LastRun        *RunRecord `json:"last_run"`
	NextRun        *time.Time `json:"next_run"`
	ReadThrough    bool       `json:"read_through"`
	Link           string     `json:"link,omitempty"`
	DryRun         *bool      `json:"dry_run,omitempty"`
}

// SchedulerView is the scheduler state on the wire.
type SchedulerView struct {
	Enabled  bool   `json:"enabled"`
	Interval string `json:"interval"`
}

// Overview is the body of GET /api/v1/cleanup.
type Overview struct {
	Targets   []TargetView  `json:"targets"`
	Scheduler SchedulerView `json:"scheduler"`
}

// ModeOf resolves the mode of a target from its runtime setting.
func (c *Center) ModeOf(ctx context.Context, m Meta) Mode {
	switch Mode(c.cfg.Settings.String(ctx, ModeKey(m.ID))) {
	case ModeOff:
		return ModeOff
	case ModePreview:
		return ModePreview
	case ModeOn:
		return ModeOn
	}
	return DefaultMode(m.Origin)
}

func (c *Center) interval(ctx context.Context) time.Duration {
	d := c.cfg.Settings.Duration(ctx, runtimesettings.KeyCleanupInterval)
	if d < runtimesettings.CleanupMinInterval {
		return runtimesettings.CleanupMinInterval
	}
	return d
}

func (c *Center) usageOf(ctx context.Context, t Target) (Usage, string) {
	id := t.Meta().ID
	now := c.cfg.Now()
	c.mu.Lock()
	e, ok := c.usage[id]
	c.mu.Unlock()
	if ok && now.Sub(e.at) < c.cfg.Limits.UsageTTL {
		return e.u, e.err
	}
	u, err := t.Measure(ctx)
	e = usageEntry{u: u, at: now}
	if err != nil {
		e.err = truncate(err.Error())
	}
	c.mu.Lock()
	c.usage[id] = e
	c.mu.Unlock()
	return e.u, e.err
}

func (c *Center) view(ctx context.Context, t Target, last map[string]store.CleanupRun) TargetView {
	m := t.Meta()
	presets := m.Presets
	if len(presets) == 0 {
		presets = runtimesettings.RetentionPresets
	}
	v := TargetView{
		ID: m.ID, Category: string(m.Category), Title: m.Title, TitleKey: m.TitleKey, Description: m.Description, DescriptionKey: m.DescKey,
		PolicyKey: m.PolicySetting, Origin: string(m.Origin), Destructive: m.Destructive, Floor: m.Floor, Presets: presets,
		ReadThrough: m.ReadOnly, Link: m.Link,
	}
	v.Usage, v.UsageError = c.usageOf(ctx, t)
	if mg, ok := t.(Managed); ok {
		if st, err := mg.State(ctx); err == nil {
			v.Days, v.Mode = st.Days, string(st.Mode)
			dry := st.DryRun
			v.DryRun = &dry
		} else {
			v.Mode = string(ModeOff)
		}
	} else {
		v.Days = c.cfg.Settings.Int(ctx, m.PolicySetting)
		v.Mode = string(c.ModeOf(ctx, m))
	}
	if r, ok := last[m.ID]; ok {
		rec := recordOf(r)
		v.LastRun = &rec
	}
	c.mu.Lock()
	if at, ok := c.nextRun[m.ID]; ok && !m.ReadOnly && v.Mode != string(ModeOff) && c.started {
		v.NextRun = &at
	}
	c.mu.Unlock()
	return v
}

func (c *Center) lastRuns(ctx context.Context) (map[string]store.CleanupRun, error) {
	rows, err := c.cfg.Store.LatestCleanupRuns(ctx)
	if err != nil {
		return nil, fmt.Errorf("cleanup: last runs: %w", err)
	}
	out := make(map[string]store.CleanupRun, len(rows))
	for _, r := range rows {
		out[r.Target] = r
	}
	return out, nil
}

// List returns every target with its usage, mode and last run.
func (c *Center) List(ctx context.Context) (Overview, error) {
	last, err := c.lastRuns(ctx)
	if err != nil {
		return Overview{}, err
	}
	out := Overview{Targets: []TargetView{}}
	for _, t := range c.cfg.Registry.Targets() {
		out.Targets = append(out.Targets, c.view(ctx, t, last))
	}
	c.mu.Lock()
	started := c.started
	c.mu.Unlock()
	out.Scheduler = SchedulerView{Enabled: started && c.cfg.Settings.Bool(ctx, runtimesettings.KeyCleanupEnabled), Interval: c.interval(ctx).String()}
	return out, nil
}

// PreviewCount is how many managed targets run as dry runs now, for the first-run review.
func (c *Center) PreviewCount(ctx context.Context) int {
	n := 0
	for _, t := range c.cfg.Registry.Targets() {
		if m := t.Meta(); !m.ReadOnly && c.ModeOf(ctx, m) == ModePreview {
			n++
		}
	}
	return n
}

// Get returns one target's view.
func (c *Center) Get(ctx context.Context, id string) (TargetView, error) {
	t, ok := c.cfg.Registry.Get(id)
	if !ok {
		return TargetView{}, ErrUnknownTarget
	}
	last, err := c.lastRuns(ctx)
	if err != nil {
		return TargetView{}, err
	}
	c.mu.Lock()
	delete(c.usage, id)
	c.mu.Unlock()
	return c.view(ctx, t, last), nil
}

func (c *Center) windowFor(ctx context.Context, m Meta, days *int) (int, error) {
	if days == nil {
		d := c.cfg.Settings.Int(ctx, m.PolicySetting)
		if d <= 0 {
			return 0, ErrNoRetention
		}
		return d, nil
	}
	switch {
	case *days < 0:
		return 0, ErrInvalidDays
	case *days == 0:
		return 0, ErrNoRetention
	case m.Floor > 0 && *days < m.Floor:
		return 0, &FloorError{Key: m.PolicySetting, Floor: m.Floor}
	}
	return *days, nil
}

// Preview reports what a run with days (or the saved policy when nil) would remove, writing nothing.
func (c *Center) Preview(ctx context.Context, id string, days *int) (Impact, error) {
	t, ok := c.cfg.Registry.Get(id)
	if !ok {
		return Impact{}, ErrUnknownTarget
	}
	m := t.Meta()
	if m.ReadOnly {
		return t.Preview(ctx, 0)
	}
	d, err := c.windowFor(ctx, m, days)
	if err != nil {
		return Impact{}, err
	}
	imp, err := t.Preview(ctx, d)
	if err != nil {
		return Impact{}, fmt.Errorf("cleanup: preview %s: %w", id, err)
	}
	return imp, nil
}

// Request is one manual or API run.
type Request struct {
	// Days overrides the saved policy when set.
	Days   *int
	DryRun bool
	Opts   RunOptions
}

// Run executes a run on demand and records it. A returned error is a pre-flight refusal; a failure of the
// run itself is in Result.Err and in history.
func (c *Center) Run(ctx context.Context, id string, req Request) (RunRecord, Result, error) {
	t, ok := c.cfg.Registry.Get(id)
	if !ok {
		return RunRecord{}, Result{}, ErrUnknownTarget
	}
	m := t.Meta()
	if m.ReadOnly {
		return RunRecord{}, Result{}, ErrReadOnly
	}
	days, err := c.windowFor(ctx, m, req.Days)
	if err != nil {
		return RunRecord{}, Result{}, err
	}
	req.Opts.DryRun = req.DryRun
	if req.Opts.Trigger == "" {
		req.Opts.Trigger = TriggerManual
	}
	return c.execute(ctx, t, days, req.Opts)
}

func (c *Center) execute(ctx context.Context, t Target, days int, opts RunOptions) (RunRecord, Result, error) {
	m := t.Meta()
	if !c.acquire(m.ID) {
		return RunRecord{}, Result{}, ErrBusy
	}
	defer c.release(m.ID)
	opts.MaxRows, opts.Slice = c.cfg.Limits.MaxRows, c.cfg.Limits.Slice

	started := c.cfg.Now()
	var res Result
	if opts.DryRun {
		imp, err := t.Preview(ctx, days)
		res = Result{Rows: imp.Rows, Bytes: imp.Bytes, Err: err}
	} else {
		res = t.Run(ctx, days, opts)
	}
	finished := c.cfg.Now()

	bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancel()
	rec := store.CleanupRun{Target: m.ID, StartedAt: started, FinishedAt: finished, Days: days, DryRun: opts.DryRun,
		Rows: res.Rows, Bytes: res.Bytes, Trigger: string(opts.Trigger), Actor: opts.Actor}
	if res.Err != nil {
		rec.Error = truncate(res.Err.Error())
	}
	notify := res.Err != nil && !c.failedRecently(bg, m.ID, started)
	id, err := c.cfg.Store.InsertCleanupRun(bg, rec)
	if err != nil {
		c.cfg.Logger.Error("cleanup: record run", slog.String("target", m.ID), slog.String("error", err.Error()))
	}
	rec.ID = id
	c.invalidateUsage(m.ID)
	c.writeAudit(bg, m, rec, opts)
	c.emit(rec, m)
	if notify {
		c.sendNotice(bg, m, rec)
	}
	return recordOf(rec), res, nil
}

func (c *Center) acquire(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running[id] {
		return false
	}
	c.running[id] = true
	return true
}

func (c *Center) release(id string) {
	c.mu.Lock()
	delete(c.running, id)
	c.mu.Unlock()
}

func (c *Center) invalidateUsage(id string) {
	c.mu.Lock()
	delete(c.usage, id)
	c.mu.Unlock()
}

func (c *Center) failedRecently(ctx context.Context, id string, now time.Time) bool {
	failed, err := c.cfg.Store.CleanupFailedSince(ctx, id, now.Add(-failureNoticeWindow))
	if err != nil {
		c.cfg.Logger.Warn("cleanup: failure history unreadable", slog.String("target", id), slog.String("error", err.Error()))
		return true
	}
	return failed
}

type auditDetail struct {
	Target  string `json:"target"`
	Days    int    `json:"days"`
	DryRun  bool   `json:"dry_run"`
	Rows    int64  `json:"rows"`
	Bytes   int64  `json:"bytes"`
	Trigger string `json:"trigger"`
	Error   string `json:"error"`
}

// writeAudit records counts only, never row contents.
func (c *Center) writeAudit(ctx context.Context, m Meta, rec store.CleanupRun, opts RunOptions) {
	if c.cfg.Audit == nil {
		return
	}
	aid, err := store.NewAuditEntryID()
	if err != nil {
		c.cfg.Logger.Warn("cleanup: audit id", slog.String("target", m.ID))
		return
	}
	detail, _ := json.Marshal(auditDetail{Target: m.ID, Days: rec.Days, DryRun: rec.DryRun, Rows: rec.Rows, Bytes: rec.Bytes, Trigger: rec.Trigger, Error: rec.Error})
	actorType, clientKind, method := opts.ActorType, "api", http.MethodPost
	if actorType == "" {
		actorType, clientKind, method = auditActorSystem, "system", http.MethodDelete
	}
	status := http.StatusOK
	if rec.Error != "" {
		status = http.StatusInternalServerError
	}
	entry := store.AuditEntry{
		ID: aid, ActorType: actorType, ActorID: firstNonEmpty(opts.ActorID, "cleanup"), ActorName: firstNonEmpty(opts.Actor, "cleanup center"),
		Ability: auditAbilityRoot, Method: method, Path: "/api/v1/cleanup/" + m.ID + "/run", StatusCode: status, RemoteAddr: opts.RemoteAddr,
		CreatedAt: store.FormatAuditTime(c.cfg.Now()), ClientKind: clientKind, Action: AuditActionRun, Detail: string(detail),
	}
	if err := c.cfg.Audit.SaveAuditEntry(ctx, entry); err != nil {
		c.cfg.Logger.Warn("cleanup: save audit row", slog.String("target", m.ID), slog.String("error", err.Error()))
	}
}

func (c *Center) emit(rec store.CleanupRun, m Meta) {
	if c.cfg.Emit == nil {
		return
	}
	data := map[string]any{
		"run_id": rec.ID, "target": m.ID, "days": rec.Days, "dry_run": rec.DryRun, "rows": rec.Rows, "bytes": rec.Bytes, "trigger": rec.Trigger,
	}
	if rec.Error != "" {
		data["error"] = rec.Error
		c.cfg.Emit(EventFailed, data)
		return
	}
	c.cfg.Emit(EventRun, data)
}

func (c *Center) sendNotice(ctx context.Context, m Meta, rec store.CleanupRun) {
	if c.cfg.Notify == nil {
		return
	}
	text := fmt.Sprintf("Cleanup of %s failed: %s", m.Title, rec.Error)
	if err := c.cfg.Notify(ctx, text); err != nil {
		c.cfg.Logger.Warn("cleanup: failure notice not delivered", slog.String("target", m.ID), slog.String("error", err.Error()))
	}
}

// RunsFilter narrows Runs.
type RunsFilter struct {
	Target string
	Since  time.Time
	Until  time.Time
	DryRun *bool
	Status string
	Limit  int
	Cursor int64
}

// RunsPage is one page of history, newest first. NextCursor is 0 on the last page.
type RunsPage struct {
	Runs       []RunRecord
	NextCursor int64
}

// Runs lists history. It reads one extra row to know whether another page exists.
func (c *Center) Runs(ctx context.Context, f RunsFilter) (RunsPage, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	rows, err := c.cfg.Store.ListCleanupRuns(ctx, store.CleanupRunFilter{Target: f.Target, Since: f.Since, Until: f.Until, DryRun: f.DryRun, Status: f.Status, Limit: limit + 1, BeforeID: f.Cursor})
	if err != nil {
		return RunsPage{}, fmt.Errorf("cleanup: list runs: %w", err)
	}
	page := RunsPage{Runs: []RunRecord{}}
	for i, r := range rows {
		if i == limit {
			page.NextCursor = rows[limit-1].ID
			break
		}
		page.Runs = append(page.Runs, recordOf(r))
	}
	return page, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func truncate(s string) string {
	if len(s) > maxErrorChars {
		return s[:maxErrorChars]
	}
	return s
}
