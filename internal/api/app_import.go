package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/appimport"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// App import session steps, in order.
const (
	appImportStepInventory = "inventory"
	appImportStepPreflight = "preflight"
	appImportStepStage     = "stage"
	appImportStepVerify    = "verify"
	appImportStepVolumes   = "volumes"
	appImportStepCutover   = "cutover"
)

var appImportSteps = []string{appImportStepInventory, appImportStepPreflight, appImportStepStage, appImportStepVerify, appImportStepVolumes, appImportStepCutover}

const (
	envAppImportReadyTimeout = "APP_MIGRATE_APP_READY_TIMEOUT"
	maxAppImportBody         = 64 << 10
	appImportIDPrefix        = "appimp-"
	appImportSourceNoneID    = "\x00none"
)

// AppImportStore is the store surface the guided app import routes need.
type AppImportStore interface {
	CreateAppImportSession(ctx context.Context, s store.AppImportSession, items []store.AppImportItem) error
	GetAppImportSession(ctx context.Context, id string) (store.AppImportSession, error)
	ListAppImportSessions(ctx context.Context) ([]store.AppImportSession, error)
	UpdateAppImportSession(ctx context.Context, s store.AppImportSession, now time.Time) error
	DeleteAppImportSession(ctx context.Context, id string) error
	ListAppImportItems(ctx context.Context, sessionID string) ([]store.AppImportItem, error)
	SaveAppImportItem(ctx context.Context, it store.AppImportItem, now time.Time) error
}

// appImportLive is what is held only in memory: the source credential and
// the last discovery, which contains env values.
type appImportLive struct {
	req     platformImportRequest
	disc    *platformimport.Discovery
	expires time.Time
}

type appImportState struct {
	mu      sync.Mutex
	live    map[string]*appImportLive
	running map[string]bool
}

func newAppImportState() *appImportState {
	return &appImportState{live: map[string]*appImportLive{}, running: map[string]bool{}}
}

func (s *appImportState) set(id string, req platformImportRequest, disc *platformimport.Discovery) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.live[id] = &appImportLive{req: req, disc: disc, expires: time.Now().Add(envDuration(envHubPasswordTTL, 2*time.Hour))}
}

func (s *appImportState) get(id string) *appImportLive {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.live[id]
	if !ok || time.Now().After(l.expires) {
		delete(s.live, id)
		return nil
	}
	return l
}

func (s *appImportState) forget(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.live, id)
}

func (s *appImportState) begin(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[id] {
		return false
	}
	s.running[id] = true
	return true
}

func (s *appImportState) end(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, id)
}

func (s *appImportState) isRunning(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running[id]
}

// appImportRecord is the JSON kept in an item's entry_json: the inventory
// row and which variables a mapping rewrote. It never holds a value.
type appImportRecord struct {
	Entry     appimport.Entry           `json:"entry"`
	Rewritten []appimport.AppliedChange `json:"rewritten,omitempty"`
}

func decodeAppImportRecord(raw string) appImportRecord {
	var r appImportRecord
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return appImportRecord{}
	}
	return r
}

func sourceBase(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Scheme + "://" + u.Host
}

// appImportContext collects what already exists here so databases the
// operator moved earlier are recognized and suggested as mapping targets.
func (rt *Router) appImportContext(ctx context.Context) (appimport.Context, map[string]string, error) {
	targets := map[string]string{}
	hosts := map[string]string{}
	managedHosts := map[string]string{}
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return appimport.Context{}, nil, fmt.Errorf("list databases: %w", err)
	}
	for _, d := range dbs {
		targets[strings.ToLower(d.Name)] = d.Name
		h := application.DatabaseHost(d.Name, rt.meshZone)
		hosts[d.Name] = h
		managedHosts[h] = d.Name
	}
	if rt.externalDatabases != nil {
		ext, err := rt.externalDatabases.ListExternalDatabases(ctx)
		if err != nil {
			return appimport.Context{}, nil, fmt.Errorf("list external databases: %w", err)
		}
		for _, d := range ext {
			if _, taken := targets[strings.ToLower(d.Name)]; !taken {
				targets[strings.ToLower(d.Name)] = d.Name
				hosts[d.Name] = d.Host
			}
		}
	}
	if rt.migrationHub != nil {
		sessions, err := rt.migrationHub.ListMigrationSessions(ctx)
		if err != nil {
			return appimport.Context{}, nil, fmt.Errorf("list hub sessions: %w", err)
		}
		for _, s := range sessions {
			items, err := rt.migrationHub.ListMigrationItems(ctx, s.ID)
			if err != nil {
				return appimport.Context{}, nil, fmt.Errorf("list hub items: %w", err)
			}
			for _, it := range items {
				if it.Status == store.HubItemVerified && it.TargetName != "" {
					targets[strings.ToLower(it.SourceDB)] = it.TargetName
					if _, ok := hosts[it.TargetName]; !ok {
						hosts[it.TargetName] = application.DatabaseHost(it.TargetName, rt.meshZone)
					}
				}
			}
		}
	}
	return appimport.Context{Targets: targets, TargetHost: func(n string) string { return hosts[n] }}, managedHosts, nil
}

func (rt *Router) targetFreeBytes() int64 {
	if rt.dataDir == "" {
		return -1
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(rt.dataDir, &st); err != nil {
		return -1
	}
	return int64(st.Bavail) * int64(st.Bsize) //nolint:gosec // statfs fields are non-negative in practice
}

// appImportCalc is one evaluation of a session against the live discovery.
type appImportCalc struct {
	inv       appimport.Inventory
	plan      *platformimport.Plan
	pre       appimport.PreflightResult
	names     map[string]string
	rewritten map[string][]appimport.AppliedChange
	connect   map[string][]string
	suggested []appimport.Mapping
}

func toMappings(in []store.AppImportMapping) []appimport.Mapping {
	out := make([]appimport.Mapping, 0, len(in))
	for _, m := range in {
		out = append(out, appimport.Mapping{From: m.From, To: m.To})
	}
	return out
}

// calcAppImport evaluates the plan, preflight and env rewrite for the
// selected items. It reads source env from live, in memory only.
func (rt *Router) calcAppImport(ctx context.Context, sess store.AppImportSession, items []store.AppImportItem, live *appImportLive) (*appImportCalc, error) {
	ictx, managedHosts, err := rt.appImportContext(ctx)
	if err != nil {
		return nil, err
	}
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	maps := toMappings(sess.Mappings)
	inv := appimport.BuildInventory(live.disc, ictx)
	selected := map[string]bool{}
	var only []string
	for _, it := range items {
		if it.Selected {
			selected[it.SourceID] = true
			only = append(only, it.SourceID)
		}
	}
	if len(only) == 0 {
		only = []string{appImportSourceNoneID}
	}

	mod := &platformimport.Discovery{Platform: live.disc.Platform, Projects: live.disc.Projects}
	envBefore := map[string][]platformimport.Env{}
	rewritten := map[string][]appimport.AppliedChange{}
	for _, a := range live.disc.Apps {
		envBefore[a.SourceID] = a.Env
		env, changes := appimport.RewriteEnv(a.Name, a.Env, maps)
		a.Env = env
		for _, c := range changes {
			rewritten[a.SourceID] = append(rewritten[a.SourceID], appimport.AppliedChange{Key: c.Key, Count: c.Count})
		}
		mod.Apps = append(mod.Apps, a)
	}

	opts := platformimport.PlanOptions{Only: only, Collision: sess.Collision, ExistingApps: map[string]string{}, TakenApps: map[string]bool{}, ExistingDatabases: map[string]string{}}
	owners := map[string]string{}
	for _, s := range svcs {
		opts.TakenApps[s.Name] = true
		if id := s.Labels[platformimport.LabelSourceID]; id != "" {
			opts.ExistingApps[id] = s.Name
		}
		for _, d := range s.Domains {
			owners[d] = s.Name
		}
	}
	plan := platformimport.BuildPlan(mod, opts)
	names := map[string]string{}
	for _, it := range plan.Report.Items {
		if it.Kind == "app" && it.Target != "" {
			names[it.SourceID] = it.Target
		}
	}
	for src, app := range opts.ExistingApps {
		_, id, _ := strings.Cut(src, ":")
		for _, d := range live.disc.Apps {
			if d.SourceID == id {
				for _, dom := range d.Domains {
					if owners[dom] == app {
						delete(owners, dom)
					}
				}
			}
		}
	}

	calc := &appImportCalc{inv: inv, plan: plan, names: names, rewritten: rewritten, connect: map[string][]string{}}
	calc.pre = appimport.Preflight(appimport.PreflightInput{
		Inventory: inv, Selected: selected, Names: names, DomainOwners: owners, Mappings: maps, Env: envBefore,
		FreeBytes: rt.targetFreeBytes(), DiskMargin: envFloat(envHubDiskMargin, appimport.DefaultDiskMargin),
	})
	seen := map[string]bool{}
	for _, e := range inv.Entries() {
		for _, d := range e.Databases {
			if d.Target != "" && d.TargetHost != "" && !seen[d.Host] {
				seen[d.Host] = true
				calc.suggested = append(calc.suggested, appimport.Mapping{From: d.Host, To: d.TargetHost})
			}
			if !selected[e.SourceID] {
				continue
			}
			for _, m := range maps {
				if db, ok := managedHosts[m.To]; ok && strings.EqualFold(m.From, d.Host) {
					calc.connect[names[e.SourceID]] = appendUnique(calc.connect[names[e.SourceID]], db)
				}
			}
		}
	}
	sort.Slice(calc.suggested, func(i, j int) bool { return calc.suggested[i].From < calc.suggested[j].From })
	return calc, nil
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
