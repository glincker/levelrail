package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envMigrateEventsInterval  = "APP_MIGRATE_EVENTS_INTERVAL"
	envMigrateEventsHeartbeat = "APP_MIGRATE_EVENTS_HEARTBEAT"
)

// Next actions a job reports, as stable codes the dashboard and CLI each word themselves.
const (
	nextReconnect   = "reconnect"
	nextStage       = "stage"
	nextRetry       = "retry"
	nextVerify      = "verify"
	nextCopyData    = "copy_data"
	nextRoute       = "route"
	nextSafeCheck   = "safe_check"
	nextNothingLeft = "none"
)

type migrationJobItem struct {
	SourceID       string `json:"source_id"`
	Name           string `json:"name"`
	Target         string `json:"target,omitempty"`
	State          string `json:"state"`
	Reason         string `json:"reason,omitempty"`
	Retryable      bool   `json:"retryable"`
	VolumesPending int    `json:"volumes_pending,omitempty"`
}

type migrationJobDatabase struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Mismatched int    `json:"mismatched,omitempty"`
}

// migrationJobProgress is the compact, secret-free state of a migration job (an
// app import session). The SSE stream, the CLI resume and the MCP tool all read it.
type migrationJobProgress struct {
	ID        string                 `json:"id"`
	Platform  string                 `json:"platform"`
	SourceURL string                 `json:"source_url"`
	Step      string                 `json:"step"`
	Phase     string                 `json:"phase"`
	Running   bool                   `json:"running"`
	Connected bool                   `json:"connected"`
	Total     int                    `json:"total"`
	Done      int                    `json:"done"`
	Failed    int                    `json:"failed"`
	States    map[string]int         `json:"states"`
	Items     []migrationJobItem     `json:"items"`
	Databases []migrationJobDatabase `json:"databases"`
	Next      []string               `json:"next"`
	Revision  string                 `json:"revision"`
}

func failedState(s string) bool {
	return s == store.AppImportStageFailed || s == store.AppImportVerifyFailed
}

// interrupted is an item left building by a control plane that died mid-verify.
func interrupted(it store.AppImportItem, running bool) bool {
	return it.State == store.AppImportBuilding && !running
}

func (rt *Router) migrationJobProgress(ctx context.Context, sess store.AppImportSession, items []store.AppImportItem) migrationJobProgress {
	running := rt.appImportLive.isRunning(sess.ID)
	p := migrationJobProgress{ID: sess.ID, Platform: sess.Platform, SourceURL: sess.SourceURL, Step: sess.Step, Running: running,
		Connected: rt.appImportLive.get(sess.ID) != nil, States: map[string]int{}, Items: []migrationJobItem{}, Databases: []migrationJobDatabase{}}
	seenDB := map[string]bool{}
	pendingData, unverified, unrouted := false, false, false
	for _, it := range items {
		if !it.Selected {
			continue
		}
		state := it.State
		retry := failedState(state) || interrupted(it, running)
		if interrupted(it, running) {
			state = store.AppImportVerifyFailed
		}
		p.Total++
		p.States[state]++
		switch {
		case state == store.AppImportVerified || state == store.AppImportRouted:
			p.Done++
		case failedState(state):
			p.Failed++
		}
		if state == store.AppImportStaged || state == store.AppImportVerifyFailed || state == store.AppImportBuilding {
			unverified = true
		}
		if state == store.AppImportVerified && len(it.Domains) > 0 {
			unrouted = true
		}
		ji := migrationJobItem{SourceID: it.SourceID, Name: it.SourceName, Target: it.TargetName, State: state, Reason: it.Reason, Retryable: retry, VolumesPending: volumesPending(it)}
		if ji.VolumesPending > 0 && !stageable(state) {
			pendingData = true
		}
		p.Items = append(p.Items, ji)
		for _, d := range decodeAppImportRecord(it.EntryJSON).Entry.Databases {
			if d.Target == "" || seenDB[d.Target] {
				continue
			}
			seenDB[d.Target] = true
			jd := migrationJobDatabase{Name: d.Target}
			if row, found, err := rt.dataImports.GetDatabaseDataImport(ctx, d.Target); err == nil && found {
				jd.Status, jd.Mismatched = row.Status, row.Mismatched
			}
			if jd.Status != "verified" || jd.Mismatched > 0 {
				pendingData = true
			}
			p.Databases = append(p.Databases, jd)
		}
	}
	sort.Slice(p.Items, func(i, j int) bool { return p.Items[i].Name < p.Items[j].Name })
	sort.Slice(p.Databases, func(i, j int) bool { return p.Databases[i].Name < p.Databases[j].Name })
	p.Phase, p.Next = jobPhase(p, unverified, pendingData, unrouted)
	p.Revision = progressRevision(p)
	return p
}

func jobPhase(p migrationJobProgress, unverified, pendingData, unrouted bool) (string, []string) {
	planned := p.States[store.AppImportPlanned] + p.States[store.AppImportRolledBack] + p.States[store.AppImportStageFailed]
	next := []string{}
	switch {
	case p.Total == 0:
		return "empty", append(next, nextNothingLeft)
	case p.Running:
		return "working", next
	}
	if !p.Connected && (planned > 0 || unverified) {
		next = append(next, nextReconnect)
	}
	if p.Failed > 0 {
		next = append(next, nextRetry)
	}
	if p.States[store.AppImportPlanned]+p.States[store.AppImportRolledBack] > 0 {
		next = append(next, nextStage)
	}
	if unverified {
		next = append(next, nextVerify)
	}
	if len(next) > 0 {
		return "attention", next
	}
	if pendingData {
		next = append(next, nextCopyData)
	}
	if unrouted {
		next = append(next, nextRoute)
	}
	next = append(next, nextSafeCheck)
	return "ready", next
}

func progressRevision(p migrationJobProgress) string {
	p.Revision = ""
	raw, _ := json.Marshal(p)
	h := fnv.New64a()
	_, _ = h.Write(raw)
	return fmt.Sprintf("%x", h.Sum64())
}

// handleAppImportProgress handles GET .../sessions/{id}/progress: a compact snapshot a
// script or an agent can poll without pulling the whole session.
func (rt *Router) handleAppImportProgress(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, rt.migrationJobProgress(r.Context(), sess, items))
}

func migrationStreamResource(*http.Request) string { return "" }

// handleAppImportEvents handles GET .../sessions/{id}/events: server-sent progress. It polls the
// stored state, so a stage or verify running in another request shows up as it saves each item.
func (rt *Router) handleAppImportEvents(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	flusher, ok := rt.startLiveSSE(w, r)
	if !ok {
		return
	}
	interval := durationEnv(envMigrateEventsInterval, time.Second)
	heartbeat := durationEnv(envMigrateEventsHeartbeat, 15*time.Second)
	lastRev := r.Header.Get("Last-Event-ID")
	lastWrite := time.Now()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		p := rt.migrationJobProgress(r.Context(), sess, items)
		switch {
		case p.Revision != lastRev:
			if data, err := json.Marshal(p); err == nil {
				_, _ = fmt.Fprintf(w, "id: %s\ndata: %s\n\n", p.Revision, data)
				flusher.Flush()
				lastRev, lastWrite = p.Revision, time.Now()
			}
		case time.Since(lastWrite) >= heartbeat:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
			lastWrite = time.Now()
		}
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
		var err error
		if sess, err = rt.appImports.GetAppImportSession(r.Context(), sess.ID); err != nil {
			if !errors.Is(err, r.Context().Err()) {
				rt.logger.Warn("api: app import: events stream ended", slog.String("session_id", sess.ID), slog.String("error", err.Error()))
			}
			return
		}
		if items, err = rt.appImports.ListAppImportItems(r.Context(), sess.ID); err != nil {
			return
		}
	}
}

// Failure classes of a connection test.
const (
	testCredentials = "credentials"
	testBlocked     = "blocked"
	testUnreachable = "unreachable"
)

type migrationTestResult struct {
	OK          bool     `json:"ok"`
	Platform    string   `json:"platform"`
	Failure     string   `json:"failure,omitempty"`
	Message     string   `json:"message,omitempty"`
	Apps        int      `json:"apps"`
	Databases   int      `json:"databases"`
	Unsupported int      `json:"unsupported"`
	Servers     []string `json:"servers"`
	Volumes     int      `json:"volumes"`
	Domains     int      `json:"domains"`
	Crons       int      `json:"crons"`
	Warnings    []string `json:"warnings"`
}

func classifyTestFailure(err error) string {
	var api *platformimport.APIError
	switch {
	case errors.Is(err, platformimport.ErrBlockedAddress):
		return testBlocked
	case errors.As(err, &api) && (api.Status == http.StatusUnauthorized || api.Status == http.StatusForbidden):
		return testCredentials
	}
	return testUnreachable
}

// handleAppImportTest handles POST /api/v1/migration/apps/test: it reads the source once and
// stores nothing. The credential lives only for this call and is wiped before it returns.
func (rt *Router) handleAppImportTest(w http.ResponseWriter, r *http.Request) {
	req, src, ok := rt.decodeAppImportCreate(w, r)
	if !ok {
		return
	}
	defer req.wipeSecrets()
	res := migrationTestResult{Platform: req.Platform, Servers: []string{}, Warnings: []string{}}
	disc, err := src.Discover(r.Context())
	if err != nil {
		res.Failure = classifyTestFailure(err)
		res.Message = envredact.Secrets(err.Error(), req.Token)
		rt.logger.Warn("api: migration connection test failed", slog.String("platform", req.Platform), slog.String("failure", res.Failure))
		writeJSON(w, http.StatusOK, res)
		return
	}
	res.OK = true
	res.Apps, res.Databases, res.Unsupported = len(disc.Apps), len(disc.Databases), len(disc.Unsupported)
	res.Warnings = append(res.Warnings, disc.Warnings...)
	for _, s := range disc.Servers {
		res.Servers = append(res.Servers, s.Name)
	}
	for _, a := range disc.Apps {
		res.Volumes += len(a.Volumes)
		res.Domains += len(a.Domains)
		res.Crons += len(a.Crons)
	}
	writeJSON(w, http.StatusOK, res)
}
