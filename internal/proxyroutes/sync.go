package proxyroutes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Route targets.
const (
	TargetApp       = "app"
	TargetDashboard = "dashboard"
)

// Per-domain file states.
const (
	StateMissing = "missing"
	StateWritten = "written"
	StateStale   = "stale"
	StateError   = "error"
)

// Audit row fields for files written into an external proxy's directory.
const (
	auditActorType  = "system"
	auditActorID    = "proxy-routes"
	auditActorName  = "Managed proxy routes"
	auditClientKind = "system"
	auditMethod     = "EVENT"
	auditPathPrefix = "/api/v1/system/proxy-integration/domains/"
)

// SyncStore is the persistence the syncer reads and writes.
type SyncStore interface {
	GetProxyIntegrationSettings(ctx context.Context) (store.ProxyIntegrationSettings, error)
	GetIngressSettings(ctx context.Context) (store.IngressSettings, error)
	ListDesiredServices(ctx context.Context) ([]store.DesiredService, error)
	ListStaticSites(ctx context.Context) ([]store.StaticSite, error)
	ListProxyRouteStatus(ctx context.Context) ([]store.ProxyRouteStatus, error)
	MarkProxyRouteWritten(ctx context.Context, domain, target, fileName string, at time.Time, changed bool) error
	MarkProxyRouteFailed(ctx context.Context, domain, target, lastError string) error
	DeleteProxyRouteStatus(ctx context.Context, domain string) error
	SaveAuditEntry(ctx context.Context, e store.AuditEntry) error
}

// Target is one domain this instance serves and where it goes.
type Target struct {
	Domain string
	Kind   string
	App    string
}

// Key is the target column stored per domain ("app:web" or "dashboard").
func (t Target) Key() string {
	if t.Kind == TargetApp {
		return TargetApp + ":" + t.App
	}
	return t.Kind
}

// Syncer converges the proxy's directory on the domains this instance serves.
type Syncer struct {
	Store     SyncStore
	NS        Namespace
	Ingress   Listener
	Dashboard Listener
	// Unit is the systemd unit named in upstream drop-in fixes.
	Unit   string
	Logger *slog.Logger

	// mu serialises passes from the reconciler and the API on one directory.
	mu sync.Mutex
}

// Item is one planned route.
type Item struct {
	Target
	File     string
	Upstream string
	Content  []byte
	Err      error
}

// Plan is what the directory should hold under the current settings.
type Plan struct {
	Settings store.ProxyIntegrationSettings
	Dir      *Dir
	DirErr   error
	Items    []Item
}

// Targets lists every domain routed by this instance: app and static site
// domains, then the primary domain to the dashboard. First claim wins.
func Targets(services []store.DesiredService, sites []store.StaticSite, primary string) []Target {
	seen := map[string]bool{}
	var out []Target
	add := func(t Target) {
		key := strings.ToLower(t.Domain)
		if t.Domain == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, t)
	}
	for _, s := range services {
		if store.IsCanaryService(s.Name) {
			continue
		}
		for _, d := range s.Domains {
			add(Target{Domain: d, Kind: TargetApp, App: s.Name})
		}
	}
	for _, s := range sites {
		for _, d := range s.Domains {
			add(Target{Domain: d, Kind: TargetApp, App: s.Name})
		}
	}
	add(Target{Domain: primary, Kind: TargetDashboard})
	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	return out
}

// PathFromSettings is the upstream path implied by a saved upstream host.
func PathFromSettings(host, unit string) UpstreamPath {
	p := UpstreamPath{Host: host, Unit: unit}
	if ip := net.ParseIP(host); ip != nil {
		p.IP = host
		p.HostNetwork = ip.IsLoopback()
	}
	return p
}

// Plan reads the settings and the routed domains and renders every file.
func (s *Syncer) Plan(ctx context.Context) (Plan, error) {
	settings, err := s.Store.GetProxyIntegrationSettings(ctx)
	if err != nil {
		return Plan{}, err
	}
	ingress, err := s.Store.GetIngressSettings(ctx)
	if err != nil {
		return Plan{}, fmt.Errorf("proxyroutes: ingress settings: %w", err)
	}
	services, err := s.Store.ListDesiredServices(ctx)
	if err != nil {
		return Plan{}, fmt.Errorf("proxyroutes: services: %w", err)
	}
	sites, err := s.Store.ListStaticSites(ctx)
	if err != nil {
		return Plan{}, fmt.Errorf("proxyroutes: static sites: %w", err)
	}
	p := Plan{Settings: settings}
	if settings.DynamicDir != "" {
		p.Dir, p.DirErr = OpenDir(s.NS, settings.DynamicDir)
	} else {
		p.DirErr = errors.New("no dynamic configuration directory is set")
	}
	path := PathFromSettings(settings.UpstreamHost, s.Unit)
	for _, t := range Targets(services, sites, ingress.PrimaryDomain) {
		p.Items = append(p.Items, s.render(settings, path, t))
	}
	return p, nil
}

func (s *Syncer) render(settings store.ProxyIntegrationSettings, path UpstreamPath, t Target) Item {
	it := Item{Target: t}
	name, err := s.NS.FileName(t.Domain)
	if err != nil {
		it.Err = err
		return it
	}
	it.File = name
	l := s.Ingress
	if t.Kind == TargetDashboard {
		l = s.Dashboard
	}
	if it.Upstream, it.Err = path.Resolve(l); it.Err != nil {
		return it
	}
	it.Content, it.Err = s.NS.Render(Route{
		Domain: t.Domain, EntrypointHTTP: settings.EntrypointHTTP, EntrypointHTTPS: settings.EntrypointHTTPS,
		CertResolver: settings.CertResolver, Upstream: it.Upstream,
	})
	return it
}

// State compares an item with what is on disk.
func (p Plan) State(it Item) (string, string) {
	if it.Err != nil {
		return StateError, it.Err.Error()
	}
	if p.Dir == nil {
		return StateError, p.DirErr.Error()
	}
	path, err := p.Dir.target(it.File)
	if err != nil {
		return StateError, err.Error()
	}
	body, err := p.Dir.read(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return StateMissing, ""
	case errors.Is(err, ErrForeignFile):
		return StateError, it.File + " exists but was not written by this instance, so it is left alone"
	case err != nil:
		return StateError, err.Error()
	case !bytes.Equal(body, it.Content):
		return StateStale, ""
	}
	return StateWritten, ""
}

// Report summarises one Sync pass.
type Report struct {
	Enabled bool
	Written []string
	Removed []string
	Failed  map[string]string
}

// Sync converges the directory: writes missing or different files, keeps
// files whose route cannot be rendered right now, and removes files no
// longer routed. With the integration off it only removes its own files.
func (s *Syncer) Sync(ctx context.Context) (Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.Plan(ctx)
	if err != nil {
		return Report{}, err
	}
	rep := Report{Enabled: p.Settings.Enabled(), Failed: map[string]string{}}
	if !rep.Enabled {
		return rep, s.disable(ctx, p, &rep)
	}
	if p.Dir == nil {
		for _, it := range p.Items {
			s.markFailed(ctx, it.Target, p.DirErr.Error())
			rep.Failed[it.Domain] = p.DirErr.Error()
		}
		return rep, p.DirErr
	}
	var want []File
	keep := map[string]bool{}
	byName := map[string]Item{}
	for _, it := range p.Items {
		if it.Err != nil {
			rep.Failed[it.Domain] = it.Err.Error()
			s.markFailed(ctx, it.Target, it.Err.Error())
			if it.File != "" {
				keep[it.File] = true
			}
			continue
		}
		byName[it.File] = it
		want = append(want, File{Domain: it.Domain, Name: it.File, Content: it.Content})
	}
	res, err := Apply(p.Dir, want, keep)
	if err != nil {
		return rep, err
	}
	now := time.Now()
	for _, o := range res.Files {
		it := byName[o.Name]
		if o.Err != nil {
			rep.Failed[o.Domain] = o.Err.Error()
			s.markFailed(ctx, it.Target, o.Err.Error())
			continue
		}
		if o.Changed {
			rep.Written = append(rep.Written, o.Domain)
			s.audit(ctx, store.AuditActionProxyRouteWrite, o.Domain)
		}
		if err := s.Store.MarkProxyRouteWritten(ctx, o.Domain, it.Key(), o.Name, now, o.Changed); err != nil {
			s.warn(ctx, "mark written", o.Domain, err)
		}
	}
	s.recordRemovals(ctx, res.Removed, &rep)
	s.forgetUnrouted(ctx, p.Items)
	if len(rep.Failed) > 0 {
		return rep, fmt.Errorf("%d of %d managed routes could not be written", len(rep.Failed), len(p.Items))
	}
	return rep, nil
}

func (s *Syncer) disable(ctx context.Context, p Plan, rep *Report) error {
	if p.Dir != nil {
		res, err := Apply(p.Dir, nil, nil)
		if err != nil {
			return err
		}
		s.recordRemovals(ctx, res.Removed, rep)
	}
	s.forgetUnrouted(ctx, nil)
	return nil
}

func (s *Syncer) recordRemovals(ctx context.Context, removed []Removal, rep *Report) {
	for _, r := range removed {
		if r.Err != nil {
			rep.Failed[r.Domain] = r.Err.Error()
			continue
		}
		rep.Removed = append(rep.Removed, r.Domain)
		s.audit(ctx, store.AuditActionProxyRouteRemove, r.Domain)
	}
}

func (s *Syncer) forgetUnrouted(ctx context.Context, items []Item) {
	routed := map[string]bool{}
	for _, it := range items {
		routed[it.Domain] = true
	}
	rows, err := s.Store.ListProxyRouteStatus(ctx)
	if err != nil {
		s.warn(ctx, "list status", "", err)
		return
	}
	for _, r := range rows {
		if !routed[r.Domain] {
			if err := s.Store.DeleteProxyRouteStatus(ctx, r.Domain); err != nil {
				s.warn(ctx, "forget status", r.Domain, err)
			}
		}
	}
}

func (s *Syncer) markFailed(ctx context.Context, t Target, msg string) {
	if err := s.Store.MarkProxyRouteFailed(ctx, t.Domain, t.Key(), msg); err != nil {
		s.warn(ctx, "mark failed", t.Domain, err)
	}
}

func (s *Syncer) audit(ctx context.Context, action, domain string) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		s.warn(ctx, "audit id", domain, err)
		return
	}
	e := store.AuditEntry{
		ID: id, ActorType: auditActorType, ActorID: auditActorID, ActorName: auditActorName,
		Ability: action, Method: auditMethod, Path: auditPathPrefix + domain, StatusCode: http.StatusOK,
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: auditClientKind, Action: action,
	}
	if err := s.Store.SaveAuditEntry(ctx, e); err != nil {
		s.warn(ctx, "audit", domain, err)
	}
}

func (s *Syncer) warn(ctx context.Context, op, domain string, err error) {
	if s.Logger != nil {
		s.Logger.WarnContext(ctx, "proxy routes: "+op+" failed", slog.String("domain", domain), slog.String("error", err.Error()))
	}
}
