package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/extdb"
	"github.com/GLINCKER/levelrail/internal/store"
)

// hosted reports whether the request comes from the hosted database flow,
// which probes before it saves.
func (req externalDatabaseRequest) hosted() bool {
	return req.URL != "" || req.Provider != ""
}

// applyConnectionURL fills fields the request left empty from its connection
// string. The password is taken from the string only when none was sent.
func (req *externalDatabaseRequest) applyConnectionURL() error {
	if req.Provider != "" {
		if _, ok := extdb.ProviderByID(req.Provider); !ok {
			return errUnknownProvider
		}
	}
	if req.URL == "" {
		return nil
	}
	p, err := extdb.ParseURL(req.URL)
	if err != nil {
		return err
	}
	req.URL = ""
	fill := func(dst *string, v string) {
		if *dst == "" {
			*dst = v
		}
	}
	fill(&req.Engine, p.Engine)
	fill(&req.Host, p.Host)
	fill(&req.Username, p.User)
	fill(&req.Password, p.Password)
	fill(&req.Database, p.Database)
	fill(&req.AuthDatabase, p.AuthDatabase)
	fill(&req.TLSMode, p.TLSMode)
	if req.Port == 0 {
		req.Port = p.Port
	}
	if req.Provider == "" {
		req.Provider = p.Provider
	}
	if req.Provider == "" {
		req.Provider = extdb.ProviderGeneric
	}
	if req.Name == "" {
		req.Name = extdb.SuggestName(req.Provider, req.Host)
	}
	return nil
}

var errUnknownProvider = errors.New("unknown provider")

// hostedSaveBlocked is the 422 body when the probe fails and the owner has
// not chosen to save anyway.
type hostedSaveBlocked struct {
	Error string       `json:"error"`
	Code  string       `json:"code"`
	Probe extdb.Result `json:"probe"`
}

const hostedProbeBudget = 3 * time.Minute

// hostedPreflight probes before anything is stored. It returns blocked=true
// after writing the response when the probe failed and save_anyway is off.
func (rt *Router) hostedPreflight(w http.ResponseWriter, r *http.Request, req externalDatabaseRequest, c extdb.Conn) (extdb.Result, bool) {
	ctx, cancel := context.WithTimeout(r.Context(), hostedProbeBudget)
	defer cancel()
	res, err := rt.externalProbe(ctx, req.Name, req.NodeID, c)
	if err != nil {
		res = extdb.Result{Status: store.ExternalHealthUnknown, Reason: c.Scrub(err.Error())}
	}
	ok := res.Status == extdb.StatusReachable || res.Status == extdb.StatusSlow
	if !ok && !req.SaveAnyway {
		writeJSON(w, http.StatusUnprocessableEntity, hostedSaveBlocked{
			Error: "the connection test failed; fix the details or choose save anyway",
			Code:  "probe_failed",
			Probe: res,
		})
		return res, true
	}
	return res, false
}

func (rt *Router) recordExternalHealth(ctx context.Context, name string, res extdb.Result) {
	if err := rt.externalDatabases.SetExternalDatabaseHealth(ctx, name, res.Status, res.Reason, res.LatencyMs, time.Now()); err != nil {
		rt.logger.Warn("api: record external database health failed", slog.String("name", name), slog.String("error", err.Error()))
	}
}

// handleListExternalDatabaseProviders handles
// GET /api/v1/external-databases/providers.
func (rt *Router) handleListExternalDatabaseProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, extdb.Providers())
}

type parseURLRequest struct {
	URL string `json:"url"`
}

// parsedConnection is the parse response. It has no password: the caller
// keeps the string and sends it again with the save request.
type parsedConnection struct {
	Engine        string `json:"engine"`
	Host          string `json:"host"`
	Port          int    `json:"port,omitempty"`
	Username      string `json:"username,omitempty"`
	Database      string `json:"database,omitempty"`
	AuthDatabase  string `json:"auth_database,omitempty"`
	TLSMode       string `json:"tls_mode"`
	Provider      string `json:"provider"`
	HasPassword   bool   `json:"has_password"`
	SuggestedName string `json:"suggested_name"`
}

// handleParseExternalDatabaseURL handles POST /api/v1/external-databases/parse.
// It only splits the string: nothing is resolved, dialed or stored.
func (rt *Router) handleParseExternalDatabaseURL(w http.ResponseWriter, r *http.Request) {
	var req parseURLRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxExternalDatabaseBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, err := extdb.ParseURL(strings.TrimSpace(req.URL))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	provider := p.Provider
	if provider == "" {
		provider = extdb.ProviderGeneric
	}
	writeJSON(w, http.StatusOK, parsedConnection{
		Engine: p.Engine, Host: p.Host, Port: p.Port, Username: p.User, Database: p.Database, AuthDatabase: p.AuthDatabase,
		TLSMode: p.TLSMode, Provider: provider, HasPassword: p.Password != "", SuggestedName: extdb.SuggestName(provider, p.Host),
	})
}
