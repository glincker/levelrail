package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/standalone"
)

// Stream message types sent by GET /api/v1/domains/{domain}/diagnose/stream.
const (
	diagnoseEventCheck = "check"
	diagnoseEventDone  = "done"
)

type diagnoseEvent struct {
	Type   string               `json:"type"`
	Check  *domaindoctor.Check  `json:"check,omitempty"`
	Report *domaindoctor.Report `json:"report,omitempty"`
}

// diagnoseOwner picks the app (or the standalone scope) that serves domain
// and that the caller may read. An empty app means a response was written.
func (rt *Router) diagnoseOwner(w http.ResponseWriter, r *http.Request) (app, domain string) {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(r.PathValue("domain")), "."))
	if domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required")
		return "", ""
	}
	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: domain diagnose: visibility", err)
		return "", ""
	}
	owners, err := rt.domainOwners(r.Context())
	if err != nil {
		rt.internalError(w, "api: domain diagnose: owners", err)
		return "", ""
	}
	for _, name := range owners[domain] {
		if canSee(name) {
			return name, domain
		}
	}
	if owns, err := rt.standaloneOwns(r.Context(), domain); err == nil && owns && canSee(standalone.Scope) {
		return standalone.Scope, domain
	}
	writeError(w, http.StatusNotFound, "domain not found; add it to an app first, then diagnose it")
	return "", ""
}

// handleDomainDiagnose handles POST /api/v1/domains/{domain}/diagnose: the
// domain doctor without needing to know which app owns the domain.
func (rt *Router) handleDomainDiagnose(w http.ResponseWriter, r *http.Request) {
	app, domain := rt.diagnoseOwner(w, r)
	if app == "" {
		return
	}
	rep, status, msg := rt.runDomainDoctor(r.Context(), app, domain, nil)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// domainDiagnoseResource scopes the stream reauth to the base read ability, not one resource.
func domainDiagnoseResource(*http.Request) string { return "" }

// handleDomainDiagnoseStream handles GET /api/v1/domains/{domain}/diagnose/stream.
// Each check is sent the moment it finishes, then one done message carries
// the ordered report, so the dashboard shows DNS results before the slow
// port and certificate probes return.
func (rt *Router) handleDomainDiagnoseStream(w http.ResponseWriter, r *http.Request) {
	app, domain := rt.diagnoseOwner(w, r)
	if app == "" {
		return
	}
	flusher, ok := startSSE(w)
	if !ok {
		return
	}
	send := func(ev diagnoseEvent) {
		data, err := json.Marshal(ev)
		if err != nil {
			rt.logger.Error("api: domain diagnose: marshal event", slog.String("error", err.Error()), slog.String("domain", domain))
			return
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
	rep, status, msg := rt.runDomainDoctor(r.Context(), app, domain, func(c domaindoctor.Check) {
		send(diagnoseEvent{Type: diagnoseEventCheck, Check: &c})
	})
	if status != 0 {
		rt.logger.Warn("api: domain diagnose stream failed", slog.Int("status", status), slog.String("domain", domain), slog.String("reason", msg))
		return
	}
	send(diagnoseEvent{Type: diagnoseEventDone, Report: &rep})
}
