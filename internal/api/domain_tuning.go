package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DomainTuningStore is the store surface .../domains/{domain}/tuning needs.
type DomainTuningStore interface {
	GetDomainTuning(ctx context.Context, domain string) (store.DomainTuning, bool, error)
	SetDomainTuning(ctx context.Context, t store.DomainTuning) error
	DeleteDomainTuning(ctx context.Context, domain string) error
}

// domainTuningPresets are the use-case labels the dashboard and CLI offer.
// The label is display only: the fields below are what the ingress renders.
var domainTuningPresets = map[string]bool{"": true, "api": true, "uploads": true, "streaming": true, "websocket": true, "custom": true}

// domainTuningResource is the wire shape for GET/PUT/DELETE .../tuning. A 0
// or false field keeps the ingress default. The bound fields are live
// runtime settings a save is checked against.
type domainTuningResource struct {
	Domain                       string     `json:"domain"`
	Configured                   bool       `json:"configured"`
	Effective                    bool       `json:"effective"`
	Preset                       string     `json:"preset"`
	Compression                  bool       `json:"compression"`
	Streaming                    bool       `json:"streaming"`
	WebSocket                    bool       `json:"websocket"`
	DialTimeoutSeconds           int        `json:"dial_timeout_seconds"`
	ResponseHeaderTimeoutSeconds int        `json:"response_header_timeout_seconds"`
	StreamTimeoutSeconds         int        `json:"stream_timeout_seconds"`
	StreamCloseDelaySeconds      int        `json:"stream_close_delay_seconds"`
	MinTimeoutSeconds            int        `json:"min_timeout_seconds"`
	MaxTimeoutSeconds            int        `json:"max_timeout_seconds"`
	MaxStreamSeconds             int        `json:"max_stream_seconds"`
	UpdatedAt                    *time.Time `json:"updated_at,omitempty"`
}

type setDomainTuningRequest struct {
	Preset                       string `json:"preset"`
	Compression                  bool   `json:"compression"`
	Streaming                    bool   `json:"streaming"`
	WebSocket                    bool   `json:"websocket"`
	DialTimeoutSeconds           int    `json:"dial_timeout_seconds"`
	ResponseHeaderTimeoutSeconds int    `json:"response_header_timeout_seconds"`
	StreamTimeoutSeconds         int    `json:"stream_timeout_seconds"`
	StreamCloseDelaySeconds      int    `json:"stream_close_delay_seconds"`
}

type domainTuningBounds struct{ min, max, maxStream int }

func (rt *Router) domainTuningBounds(ctx context.Context) domainTuningBounds {
	b := domainTuningBounds{
		min:       rt.runtimeSettings.Limit(ctx, runtimesettings.KeyDomainTuningMinTimeoutSeconds),
		max:       rt.runtimeSettings.Limit(ctx, runtimesettings.KeyDomainTuningMaxTimeoutSeconds),
		maxStream: rt.runtimeSettings.Limit(ctx, runtimesettings.KeyDomainTuningMaxStreamSeconds),
	}
	b.min = min(b.min, b.max, b.maxStream)
	return b
}

func (rt *Router) toDomainTuningResource(ctx context.Context, domain string, t store.DomainTuning, found bool) domainTuningResource {
	b := rt.domainTuningBounds(ctx)
	res := domainTuningResource{Domain: domain, MinTimeoutSeconds: b.min, MaxTimeoutSeconds: b.max, MaxStreamSeconds: b.maxStream}
	if !found {
		return res
	}
	res.Configured = true
	res.Effective = !rt.tuningDropped(ctx, domain, t.UpdatedAt)
	res.Preset, res.Compression, res.Streaming, res.WebSocket = t.Preset, t.Compression, t.Streaming, t.WebSocket
	res.DialTimeoutSeconds, res.ResponseHeaderTimeoutSeconds = t.DialTimeoutSeconds, t.ResponseHeaderTimeoutSeconds
	res.StreamTimeoutSeconds, res.StreamCloseDelaySeconds = t.StreamTimeoutSeconds, t.StreamCloseDelaySeconds
	if !t.UpdatedAt.IsZero() {
		u := t.UpdatedAt
		res.UpdatedAt = &u
	}
	return res
}

// tuningDropped reports whether the last ingress pass after this save had to
// leave the domain's tuning out. Unknown or older evidence counts as applied.
func (rt *Router) tuningDropped(ctx context.Context, domain string, savedAt time.Time) bool {
	reader, ok := rt.domainTuning.(domainConditionsReader)
	if !ok {
		return false
	}
	conds, at, known, err := reader.ListDomainConditions(ctx)
	if err != nil || !known || at.Before(savedAt) {
		return false
	}
	for _, c := range conds {
		if c.Domain == domain && c.Code == store.DomainCondTuningDropped {
			return true
		}
	}
	return false
}

func checkTuningSeconds(name string, v, lo, hi int) error {
	if v != 0 && (v < lo || v > hi) {
		return fmt.Errorf("%s must be 0 (ingress default) or between %d and %d", name, lo, hi)
	}
	return nil
}

// validateDomainTuning accepts only known presets and bounded numbers; no
// field is ever copied into the config as text.
func validateDomainTuning(domain string, req setDomainTuningRequest, b domainTuningBounds) (store.DomainTuning, error) {
	if !domainTuningPresets[req.Preset] {
		return store.DomainTuning{}, errors.New("preset must be one of api, uploads, streaming, websocket or custom")
	}
	// Compression buffers output, which breaks event streams and is pointless on upgraded sockets.
	if req.Compression && (req.Streaming || req.WebSocket) {
		return store.DomainTuning{}, errors.New("compression cannot be combined with streaming or websocket")
	}
	if !req.WebSocket && (req.StreamTimeoutSeconds != 0 || req.StreamCloseDelaySeconds != 0) {
		return store.DomainTuning{}, errors.New("stream_timeout_seconds and stream_close_delay_seconds need websocket on")
	}
	checks := []error{
		checkTuningSeconds("dial_timeout_seconds", req.DialTimeoutSeconds, b.min, b.max),
		checkTuningSeconds("response_header_timeout_seconds", req.ResponseHeaderTimeoutSeconds, b.min, b.max),
		checkTuningSeconds("stream_close_delay_seconds", req.StreamCloseDelaySeconds, b.min, b.max),
		checkTuningSeconds("stream_timeout_seconds", req.StreamTimeoutSeconds, b.min, b.maxStream),
	}
	if err := errors.Join(checks...); err != nil {
		return store.DomainTuning{}, err
	}
	return store.DomainTuning{
		Domain: domain, Preset: req.Preset, Compression: req.Compression, Streaming: req.Streaming, WebSocket: req.WebSocket,
		DialTimeoutSeconds: req.DialTimeoutSeconds, ResponseHeaderTimeoutSeconds: req.ResponseHeaderTimeoutSeconds,
		StreamTimeoutSeconds: req.StreamTimeoutSeconds, StreamCloseDelaySeconds: req.StreamCloseDelaySeconds,
	}, nil
}

func tuningIsEmpty(t store.DomainTuning) bool {
	return !t.Compression && !t.Streaming && !t.WebSocket && t.DialTimeoutSeconds == 0 &&
		t.ResponseHeaderTimeoutSeconds == 0 && t.StreamTimeoutSeconds == 0 && t.StreamCloseDelaySeconds == 0
}

// changedTuningKeys lists the wire names that differ, for the audit row.
func changedTuningKeys(prev, next store.DomainTuning) []string {
	var keys []string
	add := func(name string, changed bool) {
		if changed {
			keys = append(keys, name)
		}
	}
	add("preset", prev.Preset != next.Preset)
	add("compression", prev.Compression != next.Compression)
	add("streaming", prev.Streaming != next.Streaming)
	add("websocket", prev.WebSocket != next.WebSocket)
	add("dial_timeout_seconds", prev.DialTimeoutSeconds != next.DialTimeoutSeconds)
	add("response_header_timeout_seconds", prev.ResponseHeaderTimeoutSeconds != next.ResponseHeaderTimeoutSeconds)
	add("stream_timeout_seconds", prev.StreamTimeoutSeconds != next.StreamTimeoutSeconds)
	add("stream_close_delay_seconds", prev.StreamCloseDelaySeconds != next.StreamCloseDelaySeconds)
	sort.Strings(keys)
	return keys
}

// handleGetDomainTuning handles GET /api/v1/apps/{name}/domains/{domain}/tuning.
func (rt *Router) handleGetDomainTuning(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	t, found, err := rt.domainTuning.GetDomainTuning(r.Context(), domain)
	if err != nil {
		rt.internalError(w, "api: get domain tuning failed", err, slog.String("domain", domain))
		return
	}
	writeJSON(w, http.StatusOK, rt.toDomainTuningResource(r.Context(), domain, t, found))
}

// handleSetDomainTuning handles PUT .../tuning; the ingress applies it on its
// next reconcile pass. A request with every knob off clears the tuning.
func (rt *Router) handleSetDomainTuning(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	var req setDomainTuningRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()
	row, err := validateDomainTuning(domain, req, rt.domainTuningBounds(ctx))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	prev, _, err := rt.domainTuning.GetDomainTuning(ctx, domain)
	if err != nil {
		rt.internalError(w, "api: read domain tuning failed", err, slog.String("domain", domain))
		return
	}
	if tuningIsEmpty(row) {
		rt.clearDomainTuning(w, r, domain, prev)
		return
	}
	if err := rt.domainTuning.SetDomainTuning(ctx, row); err != nil {
		rt.internalError(w, "api: set domain tuning failed", err, slog.String("domain", domain))
		return
	}
	rt.auditDomainTuning(r, domain, changedTuningKeys(prev, row))
	saved, found, err := rt.domainTuning.GetDomainTuning(ctx, domain)
	if err != nil {
		rt.internalError(w, "api: read domain tuning failed", err, slog.String("domain", domain))
		return
	}
	writeJSON(w, http.StatusOK, rt.toDomainTuningResource(ctx, domain, saved, found))
}

// handleClearDomainTuning handles DELETE .../tuning. Idempotent.
func (rt *Router) handleClearDomainTuning(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	prev, _, err := rt.domainTuning.GetDomainTuning(r.Context(), domain)
	if err != nil {
		rt.internalError(w, "api: read domain tuning failed", err, slog.String("domain", domain))
		return
	}
	rt.clearDomainTuning(w, r, domain, prev)
}

func (rt *Router) clearDomainTuning(w http.ResponseWriter, r *http.Request, domain string, prev store.DomainTuning) {
	if err := rt.domainTuning.DeleteDomainTuning(r.Context(), domain); err != nil {
		rt.internalError(w, "api: clear domain tuning failed", err, slog.String("domain", domain))
		return
	}
	rt.auditDomainTuning(r, domain, changedTuningKeys(prev, store.DomainTuning{Domain: domain}))
	writeJSON(w, http.StatusOK, rt.toDomainTuningResource(r.Context(), domain, store.DomainTuning{}, false))
}

// auditDomainTuning records the changed key names (never values) beside the
// request audit, so a review shows exactly which knobs moved.
func (rt *Router) auditDomainTuning(r *http.Request, domain string, keys []string) {
	if rt.auditLog == nil || len(keys) == 0 {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityDeploy, Method: "DOMAIN_TUNING_SET",
		Path:       r.URL.Path + "#keys:" + strings.Join(keys, ","),
		StatusCode: http.StatusOK, RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: domain tuning audit save failed", slog.String("error", err.Error()), slog.String("domain", domain))
	}
}
