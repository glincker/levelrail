package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/dnsstatus"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/standalone"
	"github.com/GLINCKER/levelrail/internal/store"
)

// dnsWatchHeartbeat keeps proxies from closing an idle watch stream.
const dnsWatchHeartbeat = 25 * time.Second

const (
	dnsWatchRuleID   = "dns_watch"
	dnsWatchRuleName = "Domain is live"
)

// dnsWatchDB is the slice of the store the watcher persists through.
type dnsWatchDB interface {
	SaveDNSWatch(ctx context.Context, w store.DNSWatch) error
	DeleteDNSWatch(ctx context.Context, domain string) error
	ListDNSWatch(ctx context.Context) ([]store.DNSWatch, error)
}

type dnsWatchHistory interface {
	RecordHistory(ctx context.Context, e alerting.HistoryEntry) error
}

type dnsWatchStore struct{ db dnsWatchDB }

func (s dnsWatchStore) SaveWatch(ctx context.Context, e dnsstatus.WatchEntry) error {
	return s.db.SaveDNSWatch(ctx, store.DNSWatch{
		Domain: e.Domain, App: e.App, State: string(e.State), Since: e.Since, LastChecked: e.LastChecked,
		NextCheck: e.NextCheck, Attempts: e.Attempts, StalledAt: e.StalledAt, Stopped: e.Stopped, NotifiedAt: e.NotifiedAt,
	})
}

func (s dnsWatchStore) DeleteWatch(ctx context.Context, domain string) error {
	return s.db.DeleteDNSWatch(ctx, domain)
}

func (s dnsWatchStore) ListWatch(ctx context.Context) ([]dnsstatus.WatchEntry, error) {
	rows, err := s.db.ListDNSWatch(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dnsstatus.WatchEntry, 0, len(rows))
	for _, w := range rows {
		out = append(out, dnsstatus.WatchEntry{
			Domain: w.Domain, App: w.App, State: dnsstatus.WatchState(w.State), Since: w.Since, LastChecked: w.LastChecked,
			NextCheck: w.NextCheck, Attempts: w.Attempts, StalledAt: w.StalledAt, Stopped: w.Stopped, NotifiedAt: w.NotifiedAt,
		})
	}
	return out, nil
}

// WithDNSWatcher injects the watcher, for tests; production builds it from the store.
func WithDNSWatcher(w *dnsstatus.Watcher) Option {
	return func(rt *Router) { rt.dnsWatcher = w }
}

// dnsWatch returns the watcher, building it once when the store can persist it; nil means unsupported.
func (rt *Router) dnsWatch() *dnsstatus.Watcher {
	rt.dnsWatcherOnce.Do(func() {
		if rt.dnsWatcher != nil {
			return
		}
		db, ok := rt.domains.(dnsWatchDB)
		if !ok {
			return
		}
		rt.dnsWatcher = dnsstatus.NewWatcher(dnsstatus.WatchConfig{
			Store: dnsWatchStore{db: db}, Check: rt.dnsWatchCheck, Notify: rt.dnsWatchNotify,
			Settings: rt.dnsWatchSettings, Logger: rt.logger,
		})
	})
	return rt.dnsWatcher
}

// RunDNSWatcher resumes watched domains and polls them until ctx ends.
func (rt *Router) RunDNSWatcher(ctx context.Context) error {
	w := rt.dnsWatch()
	if w == nil {
		return nil
	}
	return w.Run(ctx)
}

func (rt *Router) dnsWatchSettings(ctx context.Context) (interval, limit time.Duration) {
	if rt.runtimeSettings == nil {
		return 15 * time.Second, 120 * time.Minute
	}
	interval = time.Duration(rt.runtimeSettings.Int(ctx, runtimesettings.KeyDomainsDNSWatchInterval)) * time.Second
	limit = time.Duration(rt.runtimeSettings.Int(ctx, runtimesettings.KeyDomainsDNSWatchMaxMins)) * time.Minute
	return interval, limit
}

// dnsWatchCheck reuses the domain check and certificate status so the watcher and the Domains page agree.
func (rt *Router) dnsWatchCheck(ctx context.Context, domain, _ string) (dnsstatus.WatchState, error) {
	resp := rt.runDomainCheckOpts(ctx, domain, rt.publicHost, false, true)
	switch resp.Status {
	case domainCheckStatusConnected:
		cert, err := rt.domainCertStatus(ctx, domain)
		if err != nil {
			return "", fmt.Errorf("certificate status: %w", err)
		}
		if cert.State == certFlowPending || cert.State == certFlowFailed {
			return dnsstatus.WatchWaitingCert, nil
		}
		return dnsstatus.WatchLive, nil
	case domainCheckStatusPropagating:
		return dnsstatus.WatchPropagating, nil
	case domainCheckStatusUnconfigured:
		return "", fmt.Errorf("public host is not configured")
	default:
		return dnsstatus.WatchWaitingDNS, nil
	}
}

// dnsWatchNotify writes an inbox entry; a unique resource id keeps a domain that goes live twice from being deduplicated.
func (rt *Router) dnsWatchNotify(ctx context.Context, e dnsstatus.WatchEntry) error {
	h, ok := rt.inbox.(dnsWatchHistory)
	if !ok {
		return nil
	}
	return h.RecordHistory(ctx, alerting.HistoryEntry{
		RuleID: dnsWatchRuleID, RuleName: dnsWatchRuleName, RuleKind: dnsWatchRuleID,
		ResourceID: fmt.Sprintf("domain:%s@%d", e.Domain, e.NotifiedAt.UnixNano()),
		App:        e.App, Severity: alerting.SeverityInfo, Event: alerting.EventResolved, Outcome: alerting.OutcomeSent,
		Detail: e.Domain + " is live: DNS points here and the certificate is ready.",
	})
}

// watchDomain registers domain for the app once it is attached, and watchRemoved stops it.
func (rt *Router) watchDomain(ctx context.Context, app string, domains ...string) {
	w := rt.dnsWatch()
	if w == nil {
		return
	}
	for _, d := range domains {
		if d == "" || strings.Contains(d, "*") {
			continue
		}
		w.Register(context.WithoutCancel(ctx), app, d)
	}
}

func (rt *Router) unwatchDomain(ctx context.Context, domains ...string) {
	w := rt.dnsWatch()
	if w == nil {
		return
	}
	for _, d := range domains {
		w.Unregister(context.WithoutCancel(ctx), d)
	}
}

// dnsWatchVisible limits entries to the apps the caller may see, like the domains list.
func (rt *Router) dnsWatchVisible(w http.ResponseWriter, r *http.Request) (func(dnsstatus.WatchEntry) bool, bool) {
	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: dns watch: visibility", err)
		return nil, false
	}
	return func(e dnsstatus.WatchEntry) bool {
		app := e.App
		if app == "" {
			app = standalone.Scope
		}
		return canSee(app)
	}, true
}

// AbilityRead: reads watcher state only. Without Accept: text/event-stream it answers a JSON list.
func (rt *Router) handleDomainsWatch(w http.ResponseWriter, r *http.Request) {
	watcher := rt.dnsWatch()
	if watcher == nil {
		writeError(w, http.StatusNotImplemented, "domain watching is not supported by this store")
		return
	}
	visible, ok := rt.dnsWatchVisible(w, r)
	if !ok {
		return
	}
	if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		out := []dnsstatus.WatchEntry{}
		for _, e := range watcher.List() {
			if visible(e) {
				out = append(out, e)
			}
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	snapshot, events, cancel := watcher.SubscribeWithSnapshot()
	defer cancel()
	defer footprint.Stream(r.Context(), "stream:domains watch")()
	flusher, ok := rt.startLiveSSE(w, r)
	if !ok {
		return
	}
	_, _ = fmt.Fprint(w, ": connected\n\n")
	for _, e := range snapshot {
		if visible(e) {
			writeWatchEvent(w, dnsstatus.WatchEvent{Kind: dnsstatus.WatchEventSnapshot, Entry: e})
		}
	}
	flusher.Flush()
	beat := time.NewTicker(dnsWatchHeartbeat)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-beat.C:
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		case ev := <-events:
			if visible(ev.Entry) {
				writeWatchEvent(w, ev)
				flusher.Flush()
			}
		}
	}
}

func writeWatchEvent(w http.ResponseWriter, ev dnsstatus.WatchEvent) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: watch\ndata: %s\n\n", data)
}

// AbilityRead: one domain's watcher snapshot, 404 when it is not watched or not visible.
func (rt *Router) handleDomainWatchSnapshot(w http.ResponseWriter, r *http.Request) {
	watcher := rt.dnsWatch()
	if watcher == nil {
		writeError(w, http.StatusNotImplemented, "domain watching is not supported by this store")
		return
	}
	visible, ok := rt.dnsWatchVisible(w, r)
	if !ok {
		return
	}
	e, found := watcher.Get(r.PathValue("domain"))
	if !found || !visible(e) {
		writeError(w, http.StatusNotFound, "domain is not being watched")
		return
	}
	writeJSON(w, http.StatusOK, e)
}
