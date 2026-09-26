// Package changes answers "what changed on this app recently" by merging the
// app event log, deploy attempts and the audit log into one ordered list.
package changes

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Env vars that tune the window and the size cap.
const (
	EnvWindow     = "APP_ALERT_CHANGE_WINDOW"
	EnvMaxEntries = "APP_ALERT_CHANGE_MAX"

	DefaultWindow     = 30 * time.Minute
	DefaultMaxEntries = 20
)

// Kind classifies a change.
type Kind string

// Change kinds. Lifecycle, maintenance and freeze are reported but never suspected.
const (
	KindDeploy      Kind = "deploy"
	KindRollback    Kind = "rollback"
	KindConfig      Kind = "config"
	KindEnv         Kind = "env"
	KindSecret      Kind = "secret"
	KindDomain      Kind = "domain"
	KindScale       Kind = "scale"
	KindLB          Kind = "loadbalancer"
	KindFreeze      Kind = "freeze"
	KindMaintenance Kind = "maintenance"
	KindLifecycle   Kind = "lifecycle"
)

// Change is one thing that happened to an app. Keys are names only, never values.
type Change struct {
	At          time.Time `json:"at"`
	Kind        Kind      `json:"kind"`
	Actor       string    `json:"actor"`
	Title       string    `json:"title"`
	Detail      string    `json:"detail,omitempty"`
	Keys        []string  `json:"keys,omitempty"`
	Ref         string    `json:"ref,omitempty"`
	LikelyCause bool      `json:"likely_cause,omitempty"`

	// noEffect marks a change that never took effect (failed or held deploy).
	noEffect bool
}

// Result is the aggregator output: newest first, capped, with the suspect flagged.
type Result struct {
	App       string        `json:"app"`
	Since     time.Time     `json:"since"`
	Until     time.Time     `json:"until"`
	Window    time.Duration `json:"-"`
	WindowSec int64         `json:"window_seconds"`
	Changes   []Change      `json:"changes"`
	Total     int           `json:"total"`
	Truncated bool          `json:"truncated,omitempty"`
}

// Suspect returns the change flagged as the likely cause, or nil.
func (r Result) Suspect() *Change {
	for i := range r.Changes {
		if r.Changes[i].LikelyCause {
			return &r.Changes[i]
		}
	}
	return nil
}

// EventSource lists app events; *store.DB satisfies it.
type EventSource interface {
	ListAppEvents(ctx context.Context, name string, before *store.AppEventCursor, after time.Time, kinds []string, limit int) ([]store.AppEvent, error)
}

// DeploySource lists deploy attempts newest first; *store.DB satisfies it.
type DeploySource interface {
	ListDeployAttempts(ctx context.Context, serviceName string) ([]store.DeployAttempt, error)
}

// AuditSource lists audit entries newest first; *store.DB satisfies it.
type AuditSource interface {
	ListAuditEntries(ctx context.Context, limit int, before *time.Time, filter store.AuditEntryFilter) ([]store.AuditEntry, error)
}

// Aggregator merges the sources. Any source may be nil.
type Aggregator struct {
	Events  EventSource
	Deploys DeploySource
	Audit   AuditSource
	Window  time.Duration
	Max     int
	Logger  *slog.Logger
}

// New builds an Aggregator with the window and cap taken from the environment.
func New(events EventSource, deploys DeploySource, audit AuditSource, logger *slog.Logger) *Aggregator {
	if logger == nil {
		logger = slog.Default()
	}
	return &Aggregator{Events: events, Deploys: deploys, Audit: audit, Window: WindowFromEnv(), Max: MaxFromEnv(), Logger: logger}
}

// WindowFromEnv reads APP_ALERT_CHANGE_WINDOW.
func WindowFromEnv() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(EnvWindow)); err == nil && d > 0 {
		return d
	}
	return DefaultWindow
}

// MaxFromEnv reads APP_ALERT_CHANGE_MAX.
func MaxFromEnv() int {
	if n, err := strconv.Atoi(os.Getenv(EnvMaxEntries)); err == nil && n > 0 {
		return n
	}
	return DefaultMaxEntries
}

const perSourceLimit = 200

// Collect returns what changed on app in the window ending at until. A source
// that fails is logged and skipped so one broken source cannot hide the rest.
func (a *Aggregator) Collect(ctx context.Context, app string, until time.Time) Result {
	window, limit := a.Window, a.Max
	if window <= 0 {
		window = DefaultWindow
	}
	if limit <= 0 {
		limit = DefaultMaxEntries
	}
	logger := a.Logger
	if logger == nil {
		logger = slog.Default()
	}
	since := until.Add(-window)
	res := Result{App: app, Since: since, Until: until, Window: window, WindowSec: int64(window.Seconds()), Changes: []Change{}}

	var all []Change
	all = append(all, a.fromEvents(ctx, logger, app, since, until)...)
	all = append(all, a.fromDeploys(ctx, logger, app, since, until)...)
	all = append(all, a.fromAudit(ctx, logger, app, since, until)...)
	return finish(res, all, limit)
}

func finish(res Result, all []Change, limit int) Result {
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].At.Equal(all[j].At) {
			return all[i].At.After(all[j].At)
		}
		return all[i].Ref > all[j].Ref
	})
	markSuspect(all)
	res.Total = len(all)
	if len(all) > limit {
		all = all[:limit]
		res.Truncated = true
	}
	res.Changes = append(res.Changes, all...)
	return res
}

// suspectKind reports whether k can plausibly alter runtime behavior.
func suspectKind(k Kind) bool {
	switch k {
	case KindDeploy, KindRollback, KindConfig, KindEnv, KindSecret, KindDomain, KindScale, KindLB:
		return true
	}
	return false
}

// markSuspect flags the newest change that took effect and can alter runtime
// behavior. all is newest first, so that is the change nearest before the alert.
func markSuspect(all []Change) {
	for i := range all {
		if suspectKind(all[i].Kind) && !all[i].noEffect {
			all[i].LikelyCause = true
			return
		}
	}
}

func inWindow(at, since, until time.Time) bool {
	return !at.Before(since) && !at.After(until)
}

func (a *Aggregator) fromEvents(ctx context.Context, logger *slog.Logger, app string, since, until time.Time) []Change {
	if a.Events == nil {
		return nil
	}
	list, err := a.Events.ListAppEvents(ctx, app, nil, since, nil, perSourceLimit)
	if err != nil {
		logger.Warn("changes: list app events failed", slog.String("app", app), slog.String("error", err.Error()))
		return nil
	}
	var out []Change
	for _, e := range list {
		if inWindow(e.CreatedAt, since, until) {
			out = append(out, eventChange(e))
		}
	}
	return out
}

func eventChange(e store.AppEvent) Change {
	kind := KindConfig
	switch e.Kind {
	case store.AppEventEnvChange:
		kind = KindEnv
	case store.AppEventSecretChange:
		kind = KindSecret
	case store.AppEventScale:
		kind = KindScale
	case store.AppEventFreezeOverride:
		kind = KindFreeze
	case store.AppEventRestart, store.AppEventSuspend, store.AppEventResume:
		kind = KindLifecycle
	case store.AppEventConfigChange:
		if len(e.Keys) == 1 && e.Keys[0] == "domains" {
			kind = KindDomain
		}
	}
	return Change{At: e.CreatedAt.UTC(), Kind: kind, Actor: e.Actor, Title: e.Title, Detail: e.Detail, Keys: e.Keys, Ref: "event:" + e.ID}
}

func (a *Aggregator) fromDeploys(ctx context.Context, logger *slog.Logger, app string, since, until time.Time) []Change {
	if a.Deploys == nil {
		return nil
	}
	attempts, err := a.Deploys.ListDeployAttempts(ctx, app)
	if err != nil {
		logger.Warn("changes: list deploy attempts failed", slog.String("app", app), slog.String("error", err.Error()))
		return nil
	}
	var out []Change
	for i, at := range attempts {
		if !inWindow(at.StartedAt, since, until) {
			continue
		}
		var older []store.DeployAttempt
		if i+1 < len(attempts) {
			older = attempts[i+1:]
		}
		out = append(out, attemptChange(at, IsRollbackAttempt(at, older)))
	}
	return out
}

// IsRollbackAttempt reports whether a redeployed an image that an attempt
// older than its immediate predecessor had already deployed. older is newest first.
func IsRollbackAttempt(a store.DeployAttempt, older []store.DeployAttempt) bool {
	if a.Source == store.DeployAttemptSourceAutoRollback {
		return true
	}
	if len(older) < 2 || older[0].Image == a.Image {
		return false
	}
	for _, o := range older[1:] {
		if o.Status == store.DeployAttemptStatusSucceeded && o.Image == a.Image {
			return true
		}
	}
	return false
}

func shortDigest(d string) string {
	d = strings.TrimPrefix(d, "sha256:")
	return d[:min(len(d), 12)]
}

func attemptChange(a store.DeployAttempt, rollback bool) Change {
	kind, verb := KindDeploy, "Deploy"
	if rollback {
		kind, verb = KindRollback, "Rollback"
	}
	actor := a.Author
	if actor == "" {
		switch a.Source {
		case store.DeployAttemptSourceWebhook:
			actor = "webhook"
		case store.DeployAttemptSourceAutoRollback:
			actor = "auto-rollback"
		default:
			actor = "manual"
		}
	}
	var detail []string
	if a.CommitSHA != "" {
		detail = append(detail, "commit "+a.CommitSHA[:min(len(a.CommitSHA), 7)])
	}
	if a.ImageDigest != "" {
		detail = append(detail, "digest "+shortDigest(a.ImageDigest))
	}
	c := Change{At: a.StartedAt.UTC(), Kind: kind, Actor: actor, Detail: strings.Join(detail, ", "), Ref: "deploy:" + a.ID}
	title := fmt.Sprintf("%s to %s", verb, imageName(a.Image))
	switch a.Status {
	case store.DeployAttemptStatusSucceeded:
	case store.DeployAttemptStatusRunning:
		title += " in progress"
	case store.DeployAttemptStatusFailed:
		title += " failed"
		c.noEffect = true
	case store.DeployAttemptStatusHeld:
		title += " held"
		c.noEffect = true
	default:
		title += " " + a.Status
		c.noEffect = true
	}
	c.Title = title
	return c
}

func imageName(image string) string {
	name, digest, ok := strings.Cut(image, "@")
	if !ok {
		return image
	}
	return name + "@sha256:" + shortDigest(digest)
}

func (a *Aggregator) fromAudit(ctx context.Context, logger *slog.Logger, app string, since, until time.Time) []Change {
	if a.Audit == nil {
		return nil
	}
	prefix := "/api/v1/apps/" + app + "/"
	var out []Change
	for _, suffix := range []string{"loadbalancer", "domains/", "deploy-freeze"} {
		entries, err := a.Audit.ListAuditEntries(ctx, perSourceLimit, nil, store.AuditEntryFilter{Search: prefix + suffix})
		if err != nil {
			logger.Warn("changes: list audit entries failed", slog.String("app", app), slog.String("error", err.Error()))
			continue
		}
		for _, e := range entries {
			if c, ok := auditChange(e, prefix, since, until); ok {
				out = append(out, c)
			}
		}
	}
	return out
}

func auditChange(e store.AuditEntry, prefix string, since, until time.Time) (Change, bool) {
	at, err := time.Parse(time.RFC3339Nano, e.CreatedAt)
	if err != nil || !inWindow(at, since, until) || e.StatusCode >= 400 {
		return Change{}, false
	}
	if e.Method != "PUT" && e.Method != "POST" && e.Method != "DELETE" {
		return Change{}, false
	}
	rest, ok := strings.CutPrefix(e.Path, prefix)
	if !ok {
		return Change{}, false
	}
	c := Change{At: at.UTC(), Actor: e.ActorName, Ref: "audit:" + e.ID}
	switch {
	case strings.HasPrefix(rest, "loadbalancer"):
		c.Kind, c.Title = KindLB, "Load balancer "+auditVerb(e.Method)
	case strings.HasSuffix(rest, "/maintenance"):
		c.Kind, c.Title = KindMaintenance, "Domain maintenance "+auditVerb(e.Method)
	case rest == "deploy-freeze":
		c.Kind, c.Title = KindFreeze, "Deploy freeze "+auditVerb(e.Method)
	default:
		return Change{}, false
	}
	return c, true
}

func auditVerb(method string) string {
	if method == "DELETE" {
		return "removed"
	}
	return "updated"
}
