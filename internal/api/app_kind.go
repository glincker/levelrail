package api

import (
	"context"
	"path"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/apiclient"
	"github.com/glincker/levelrail/internal/reconcile"
	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/kit/cronexpr"
)

// App kinds the dashboard uses to pick a card shape.
const (
	appKindWeb      = "web"
	appKindAPI      = "api"
	appKindWorker   = "worker"
	appKindCron     = "cron"
	appKindDatabase = "database"
	appKindStatic   = "static"
)

// Where a kind came from. kindSourceProbe is reserved: no probe content type
// is stored today, so nothing emits it yet.
const (
	kindSourceSpec      = "spec"
	kindSourceProbe     = "probe"
	kindSourceHeuristic = "heuristic"
)

// appKindInput is the minimum the classifier reads, so a table test needs no store.
type appKindInput struct {
	Image     string
	Port      int
	Domains   []string
	HealthURL string
	HasJobs   bool
	BuildType string
	// ContentType is the last probe content type when a caller knows it. Always empty from the list today.
	ContentType string
}

var databaseImages = map[string]bool{
	"postgres": true, "postgis": true, "mysql": true, "mariadb": true, "mongo": true, "redis": true,
	"valkey": true, "memcached": true, "clickhouse": true, "cockroach": true, "influxdb": true,
	"timescaledb": true, "couchdb": true, "cassandra": true, "etcd": true, "rabbitmq": true,
}

var apiPathPrefixes = []string{"/api", "/v1", "/v2", "/v3", "/graphql", "/rpc", "/grpc"}

var healthPathPrefixes = []string{"/health", "/healthz", "/readyz", "/livez", "/ready", "/live", "/ping", "/status", "/up"}

// classifyApp picks the card kind from declared config first and only then guesses.
func classifyApp(in appKindInput) (kind, source string) {
	hasDomain := len(in.Domains) > 0
	serves := in.Port > 0 || hasDomain
	switch {
	case in.BuildType == "static":
		return appKindStatic, kindSourceSpec
	case in.HasJobs && !serves:
		return appKindCron, kindSourceSpec
	case !hasDomain && isDatabaseImage(in.Image):
		return appKindDatabase, kindSourceHeuristic
	case !serves:
		return appKindWorker, kindSourceSpec
	}
	if ct := strings.ToLower(strings.TrimSpace(in.ContentType)); ct != "" {
		if strings.HasPrefix(ct, "text/html") {
			return appKindWeb, kindSourceProbe
		}
		return appKindAPI, kindSourceProbe
	}
	p := strings.ToLower(in.HealthURL)
	if hasPathPrefix(p, apiPathPrefixes) {
		return appKindAPI, kindSourceHeuristic
	}
	// A bare health path says little for a public host, but a port-only service has no page to show.
	if !hasDomain && hasPathPrefix(p, healthPathPrefixes) {
		return appKindAPI, kindSourceHeuristic
	}
	return appKindWeb, kindSourceHeuristic
}

func hasPathPrefix(p string, prefixes []string) bool {
	for _, pre := range prefixes {
		if p == pre || strings.HasPrefix(p, pre+"/") || strings.HasPrefix(p, pre+"?") {
			return true
		}
	}
	return false
}

func isDatabaseImage(image string) bool {
	if i := strings.LastIndexAny(image, "@"); i >= 0 {
		image = image[:i]
	}
	base := path.Base(image)
	if i := strings.Index(base, ":"); i >= 0 {
		base = base[:i]
	}
	return databaseImages[strings.ToLower(base)]
}

func kindInputFor(svc store.DesiredService, hasJobs bool) appKindInput {
	in := appKindInput{Image: svc.Image, Port: svc.Port, Domains: svc.Domains, HasJobs: hasJobs}
	if svc.Health != nil && svc.Health.Readiness != nil {
		in.HealthURL = svc.Health.Readiness.Path
	}
	return in
}

// appListSummary is the one definition of the list card data, shared with the CLI client.
type appListSummary = apiclient.AppListSummary

type enabledTaskSource interface {
	ListEnabledScheduledTasks(ctx context.Context) ([]store.ScheduledTask, error)
}

// jobSummary folds one app's enabled jobs into next run and the newest run result.
type jobSummary struct {
	count   int
	next    *time.Time
	lastAt  *time.Time
	lastRun string
}

// jobsByApp reads every enabled job in one query; nil when the store cannot list them.
func (rt *Router) jobsByApp(ctx context.Context, now time.Time) map[string]*jobSummary {
	src, ok := rt.scheduledTasks.(enabledTaskSource)
	if !ok {
		return nil
	}
	tasks, err := src.ListEnabledScheduledTasks(ctx)
	if err != nil {
		return nil
	}
	return foldJobs(tasks, now)
}

func foldJobs(tasks []store.ScheduledTask, now time.Time) map[string]*jobSummary {
	out := make(map[string]*jobSummary)
	for _, t := range tasks {
		js := out[t.ServiceName]
		if js == nil {
			js = &jobSummary{}
			out[t.ServiceName] = js
		}
		js.count++
		if sched, err := cronexpr.Parse(t.Schedule); err == nil {
			loc := time.UTC
			if l, lerr := time.LoadLocation(t.Timezone); lerr == nil && t.Timezone != "" {
				loc = l
			}
			if n := cronexpr.NextInLocation(sched, now, loc); !n.IsZero() && (js.next == nil || n.Before(*js.next)) {
				n := n
				js.next = &n
			}
		}
		if t.LastRunAt != nil && (js.lastAt == nil || t.LastRunAt.After(*js.lastAt)) {
			at := *t.LastRunAt
			js.lastAt, js.lastRun = &at, t.LastRunStatus
		}
	}
	return out
}

func buildAppSummary(kind string, svc store.DesiredService, conds []reconcile.Condition, deployedAt *time.Time, js *jobSummary) *appListSummary {
	s := &appListSummary{LastDeployAt: deployedAt}
	if kind == appKindCron {
		if js != nil {
			s.Jobs, s.NextRunAt, s.LastRunAt, s.LastRunState = js.count, js.next, js.lastAt, js.lastRun
		}
		return s
	}
	s.Replicas = svc.Replicas
	s.Ready = "unknown"
	for _, c := range conds {
		if c.Type != reconcile.ConditionTypeReady {
			continue
		}
		switch c.Status {
		case reconcile.ConditionTrue:
			s.Ready = "true"
		case reconcile.ConditionFalse:
			s.Ready = "false"
		}
	}
	return s
}
