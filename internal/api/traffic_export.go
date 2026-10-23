package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/trafficexport"
)

// TrafficExportStore is the sink surface the traffic export handlers need; *store.DB satisfies it.
type TrafficExportStore interface {
	ListTrafficExportSinks(ctx context.Context) ([]store.TrafficExportSink, error)
	GetTrafficExportSink(ctx context.Context, id string) (store.TrafficExportSink, error)
	UpsertTrafficExportSink(ctx context.Context, s store.TrafficExportSink) error
	DeleteTrafficExportSink(ctx context.Context, id string) error
}

// TrafficExportSecrets is the part of internal/secrets.Manager the handlers need. Resolve results never reach a response.
type TrafficExportSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// TrafficExporter is the running export service: reload after a config write, live status, one synthetic delivery.
type TrafficExporter interface {
	Reload(ctx context.Context) error
	Status() []trafficexport.SinkStatus
	Test(ctx context.Context, spec trafficexport.SinkSpec) trafficexport.TestResult
}

const trafficExportNameMax = 80

// trafficExportSinkResource is the wire shape of one sink. The url, token and auth_header fields are write-only: a
// response carries only the *_set booleans, and an empty value on a write keeps the stored one.
type trafficExportSinkResource struct {
	ID         string   `json:"id,omitempty"`
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Enabled    *bool    `json:"enabled,omitempty"`
	Host       string   `json:"host,omitempty"`
	TargetHost string   `json:"target_host,omitempty"`
	Fields     []string `json:"fields,omitempty"`
	IPTruncate *bool    `json:"ip_truncate,omitempty"`
	Sample     *float64 `json:"sample,omitempty"`

	URL           string `json:"url,omitempty"`
	URLSet        bool   `json:"url_set,omitempty"`
	Token         string `json:"token,omitempty"`
	TokenSet      bool   `json:"token_set,omitempty"`
	AuthHeader    string `json:"auth_header,omitempty"`
	AuthHeaderSet bool   `json:"auth_header_set,omitempty"`

	IdentityEnabled bool       `json:"identity_enabled"`
	CreatedAt       *time.Time `json:"created_at,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

type trafficExportListResponse struct {
	Sinks []trafficExportSinkResource `json:"sinks"`
}

type trafficExportStatusResponse struct {
	Sinks []trafficexport.SinkStatus `json:"sinks"`
}

func (r *trafficExportSinkResource) secretValues() map[string]string {
	out := map[string]string{}
	for slot, v := range map[string]string{
		trafficexport.SecretURL: r.URL, trafficexport.SecretToken: r.Token, trafficexport.SecretAuthHeader: r.AuthHeader,
	} {
		if v != "" {
			out[slot] = v
		}
	}
	return out
}

func (r *trafficExportSinkResource) applyFlags(set map[string]bool) {
	r.URLSet, r.TokenSet, r.AuthHeaderSet = set[trafficexport.SecretURL], set[trafficexport.SecretToken], set[trafficexport.SecretAuthHeader]
}

func toTrafficExportResource(s store.TrafficExportSink) trafficExportSinkResource {
	enabled, trunc, sample := s.Enabled, s.IPTruncate, s.Sample
	fields := s.Fields
	if fields == nil {
		fields = []string{}
	}
	return trafficExportSinkResource{
		ID: s.ID, Name: s.Name, Kind: s.Kind, Enabled: &enabled, Host: s.Host, TargetHost: s.TargetHost,
		Fields: fields, IPTruncate: &trunc, Sample: &sample, IdentityEnabled: trafficexport.HasIdentity(fields),
		CreatedAt: timePtr(s.CreatedAt), UpdatedAt: timePtr(s.UpdatedAt),
	}
}

// trafficExportSetFlags reads which secret slots a sink has stored; values are never read here.
func (rt *Router) trafficExportSetFlags(ctx context.Context, s store.TrafficExportSink) (map[string]bool, error) {
	set := map[string]bool{}
	if rt.trafficExportSecrets == nil {
		return set, nil
	}
	for _, slot := range trafficexport.SecretSlots(s.Kind) {
		ok, err := rt.trafficExportSecrets.Exists(ctx, store.TrafficExportSecretsKey(s.ID), slot)
		if err != nil {
			return nil, fmt.Errorf("check %s: %w", slot, err)
		}
		set[slot] = ok
	}
	return set, nil
}

func (rt *Router) trafficExportView(ctx context.Context, s store.TrafficExportSink) (trafficExportSinkResource, error) {
	res := toTrafficExportResource(s)
	set, err := rt.trafficExportSetFlags(ctx, s)
	if err != nil {
		return res, err
	}
	res.applyFlags(set)
	return res, nil
}

// storedTrafficExportSecrets resolves a sink's stored secret slots; a missing slot is simply absent.
func (rt *Router) storedTrafficExportSecrets(ctx context.Context, s store.TrafficExportSink) (map[string]string, error) {
	out := map[string]string{}
	for _, slot := range trafficexport.SecretSlots(s.Kind) {
		v, err := rt.trafficExportSecrets.Resolve(ctx, store.TrafficExportSecretsKey(s.ID), slot)
		switch {
		case errors.Is(err, secrets.ErrValueNotFound):
		case err != nil:
			return nil, fmt.Errorf("read %s: %w", slot, err)
		default:
			out[slot] = v
		}
	}
	return out, nil
}

func trafficExportSpec(s store.TrafficExportSink, sec map[string]string) trafficexport.SinkSpec {
	return trafficexport.SinkSpec{
		ID: s.ID, Name: s.Name, Kind: s.Kind, Enabled: s.Enabled, Host: s.Host, Fields: s.Fields,
		IPTruncate: s.IPTruncate, Sample: s.Sample, Secrets: sec,
	}
}

// targetHostOf is the hostname shown in the UI; the full URL may hold a token and is never echoed.
func targetHostOf(s store.TrafficExportSink, sec map[string]string) string {
	if s.Kind == trafficexport.SinkPostHog {
		switch strings.ToLower(strings.TrimSpace(s.Host)) {
		case "", "us":
			return "us.i.posthog.com"
		case "eu":
			return "eu.i.posthog.com"
		}
		if u, err := url.Parse(strings.TrimSpace(s.Host)); err == nil {
			return u.Hostname()
		}
		return ""
	}
	if u, err := url.Parse(strings.TrimSpace(sec[trafficexport.SecretURL])); err == nil {
		return u.Hostname()
	}
	return ""
}

// scrub removes secret values from a validation message before it reaches a client.
func scrub(msg string, sec map[string]string) string {
	for _, v := range sec {
		if v != "" {
			msg = strings.ReplaceAll(msg, v, "[redacted]")
		}
	}
	return msg
}

// apply overlays a request onto a stored sink (or a fresh one) and returns the merged secret set.
func (r *trafficExportSinkResource) apply(base store.TrafficExportSink, stored map[string]string) (store.TrafficExportSink, map[string]string) {
	out := base
	if r.Name != "" {
		out.Name = strings.TrimSpace(r.Name)
	}
	if r.Kind != "" {
		out.Kind = r.Kind
	}
	if r.Enabled != nil {
		out.Enabled = *r.Enabled
	}
	if r.IPTruncate != nil {
		out.IPTruncate = *r.IPTruncate
	}
	if r.Sample != nil {
		out.Sample = *r.Sample
	}
	if r.Fields != nil {
		out.Fields = r.Fields
	}
	if r.Host != "" {
		out.Host = r.Host
	}
	merged := map[string]string{}
	for k, v := range stored {
		merged[k] = v
	}
	for k, v := range r.secretValues() {
		merged[k] = v
	}
	out.TargetHost = targetHostOf(out, merged)
	return out, merged
}

func validateTrafficExport(s store.TrafficExportSink, sec map[string]string) error {
	if s.Name == "" {
		return errors.New("name is required")
	}
	if len(s.Name) > trafficExportNameMax || strings.ContainsAny(s.Name, "\r\n") {
		return fmt.Errorf("name must be at most %d characters and a single line", trafficExportNameMax)
	}
	allowed := map[string]bool{}
	for _, k := range trafficexport.SinkKinds() {
		allowed[k] = true
	}
	if !allowed[s.Kind] {
		return fmt.Errorf("kind must be one of %s", strings.Join(trafficexport.SinkKinds(), ", "))
	}
	for slot := range sec {
		ok := false
		for _, a := range trafficexport.SecretSlots(s.Kind) {
			ok = ok || a == slot
		}
		if !ok {
			return fmt.Errorf("%s does not apply to a %s sink", slot, s.Kind)
		}
	}
	if err := trafficexport.ValidateSpec(trafficExportSpec(s, sec)); err != nil {
		return errors.New(scrub(err.Error(), sec))
	}
	return nil
}

func (rt *Router) writeTrafficExportError(w http.ResponseWriter, msg string, err error) {
	switch {
	case errors.Is(err, store.ErrTrafficExportSinkNotFound):
		writeError(w, http.StatusNotFound, "traffic export sink not found")
	case errors.Is(err, store.ErrTrafficExportSinkInvalid):
		writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), store.ErrTrafficExportSinkInvalid.Error()+": "))
	default:
		rt.internalError(w, msg, err)
	}
}

func (rt *Router) reloadTrafficExport(ctx context.Context) {
	if rt.trafficExporter == nil {
		return
	}
	if err := rt.trafficExporter.Reload(ctx); err != nil {
		rt.logger.Warn("api: traffic export reload failed", slog.String("error", err.Error()))
	}
}

// auditTrafficExportSecrets records which credential slots changed for a sink, never their values.
func (rt *Router) auditTrafficExportSecrets(r *http.Request, sinkID string, slots []string) {
	if len(slots) == 0 || rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: traffic export audit id failed", slog.String("error", err.Error()))
		return
	}
	detail, err := json.Marshal(map[string]any{"sink_id": sinkID, "fields": slots})
	if err != nil {
		return
	}
	actorType, actorID, actorName, _ := rt.currentActor(r)
	err = rt.auditLog.SaveAuditEntry(r.Context(), store.AuditEntry{
		ID: id, ActorType: actorType, ActorID: actorID, ActorName: actorName, Ability: AbilityRoot,
		Method: r.Method, Path: r.URL.Path, StatusCode: http.StatusOK, RemoteAddr: clientIP(r),
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action: store.AuditActionTrafficExportSecretsChanged, Detail: string(detail),
	})
	if err != nil {
		rt.logger.Warn("api: traffic export audit failed", slog.String("error", err.Error()))
	}
}

// handleListTrafficExportSinks handles GET /api/v1/settings/traffic-export/sinks.
func (rt *Router) handleListTrafficExportSinks(w http.ResponseWriter, r *http.Request) {
	rows, err := rt.trafficExport.ListTrafficExportSinks(r.Context())
	if err != nil {
		rt.internalError(w, "api: list traffic export sinks failed", err)
		return
	}
	out := trafficExportListResponse{Sinks: make([]trafficExportSinkResource, 0, len(rows))}
	for _, s := range rows {
		res, err := rt.trafficExportView(r.Context(), s)
		if err != nil {
			rt.internalError(w, "api: list traffic export sinks: secrets check failed", err)
			return
		}
		out.Sinks = append(out.Sinks, res)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetTrafficExportSink handles GET /api/v1/settings/traffic-export/sinks/{id}.
func (rt *Router) handleGetTrafficExportSink(w http.ResponseWriter, r *http.Request) {
	s, err := rt.trafficExport.GetTrafficExportSink(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.writeTrafficExportError(w, "api: get traffic export sink failed", err)
		return
	}
	res, err := rt.trafficExportView(r.Context(), s)
	if err != nil {
		rt.internalError(w, "api: get traffic export sink: secrets check failed", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

const trafficExportNoKey = "traffic export is not configured on this control plane (no master key set)"

// saveTrafficExportSink writes secrets before the row, so a failed row write leaves an orphaned secret, never a row
// that looks configured but is not.
func (rt *Router) saveTrafficExportSink(w http.ResponseWriter, r *http.Request, s store.TrafficExportSink, req trafficExportSinkResource, created bool) {
	ctx := r.Context()
	svc := store.TrafficExportSecretsKey(s.ID)
	var written []string
	for slot, v := range req.secretValues() {
		if err := rt.trafficExportSecrets.SetValue(ctx, svc, slot, v); err != nil {
			rt.internalError(w, "api: save traffic export sink: set secret failed", fmt.Errorf("set %s: %w", slot, err))
			return
		}
		written = append(written, slot)
	}
	if err := rt.trafficExport.UpsertTrafficExportSink(ctx, s); err != nil {
		if created {
			_ = rt.trafficExportSecrets.DeleteAll(ctx, svc)
		}
		rt.writeTrafficExportError(w, "api: save traffic export sink failed", err)
		return
	}
	rt.auditTrafficExportSecrets(r, s.ID, written)
	rt.reloadTrafficExport(ctx)
	saved, err := rt.trafficExport.GetTrafficExportSink(ctx, s.ID)
	if err != nil {
		rt.internalError(w, "api: save traffic export sink: reload failed", err)
		return
	}
	res, err := rt.trafficExportView(ctx, saved)
	if err != nil {
		rt.internalError(w, "api: save traffic export sink: secrets check failed", err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, res)
}

// handleCreateTrafficExportSink handles POST /api/v1/settings/traffic-export/sinks. A new sink exports no visitor
// identity unless its request lists the fields.
func (rt *Router) handleCreateTrafficExportSink(w http.ResponseWriter, r *http.Request) {
	if rt.trafficExportSecrets == nil {
		writeError(w, http.StatusNotImplemented, trafficExportNoKey)
		return
	}
	var req trafficExportSinkResource
	if !decodeReq(w, r, &req) {
		return
	}
	id, err := randomOpaqueID("tex_")
	if err != nil {
		rt.internalError(w, "api: create traffic export sink: id failed", err)
		return
	}
	base := store.TrafficExportSink{ID: id, Enabled: true, IPTruncate: true, Sample: 1}
	s, sec := req.apply(base, nil)
	if err := validateTrafficExport(s, sec); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rt.saveTrafficExportSink(w, r, s, req, true)
}

// handleUpdateTrafficExportSink handles PUT /api/v1/settings/traffic-export/sinks/{id}. Omitted fields and empty
// credentials keep the stored value.
func (rt *Router) handleUpdateTrafficExportSink(w http.ResponseWriter, r *http.Request) {
	if rt.trafficExportSecrets == nil {
		writeError(w, http.StatusNotImplemented, trafficExportNoKey)
		return
	}
	ctx := r.Context()
	existing, err := rt.trafficExport.GetTrafficExportSink(ctx, r.PathValue("id"))
	if err != nil {
		rt.writeTrafficExportError(w, "api: update traffic export sink: get failed", err)
		return
	}
	var req trafficExportSinkResource
	if !decodeReq(w, r, &req) {
		return
	}
	if req.Kind != "" && req.Kind != existing.Kind {
		writeError(w, http.StatusBadRequest, "kind cannot be changed; remove the sink and add a new one")
		return
	}
	stored, err := rt.storedTrafficExportSecrets(ctx, existing)
	if err != nil {
		rt.internalError(w, "api: update traffic export sink: read secrets failed", err)
		return
	}
	s, sec := req.apply(existing, stored)
	if err := validateTrafficExport(s, sec); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rt.saveTrafficExportSink(w, r, s, req, false)
}

// handleDeleteTrafficExportSink handles DELETE /api/v1/settings/traffic-export/sinks/{id}. The row goes first so a
// refused delete never costs the credentials.
func (rt *Router) handleDeleteTrafficExportSink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	if err := rt.trafficExport.DeleteTrafficExportSink(ctx, id); err != nil {
		rt.writeTrafficExportError(w, "api: delete traffic export sink failed", err)
		return
	}
	if rt.trafficExportSecrets != nil {
		if err := rt.trafficExportSecrets.DeleteAll(ctx, store.TrafficExportSecretsKey(id)); err != nil {
			rt.logger.Warn("api: delete traffic export secrets failed", slog.String("sink", id), slog.String("error", err.Error()))
		}
	}
	rt.reloadTrafficExport(ctx)
	w.WriteHeader(http.StatusNoContent)
}

// handleTestTrafficExportSink handles POST /api/v1/settings/traffic-export/sinks/{id}/test: one synthetic record
// through that sink alone, enabled or not. A failed delivery is a 200 with ok false and an error kind.
func (rt *Router) handleTestTrafficExportSink(w http.ResponseWriter, r *http.Request) {
	if rt.trafficExporter == nil || rt.trafficExportSecrets == nil {
		writeError(w, http.StatusNotImplemented, trafficExportNoKey)
		return
	}
	s, err := rt.trafficExport.GetTrafficExportSink(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.writeTrafficExportError(w, "api: test traffic export sink: get failed", err)
		return
	}
	sec, err := rt.storedTrafficExportSecrets(r.Context(), s)
	if err != nil {
		rt.internalError(w, "api: test traffic export sink: read secrets failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.trafficExporter.Test(r.Context(), trafficExportSpec(s, sec)))
}

// handleTrafficExportStatus handles GET /api/v1/settings/traffic-export/status.
func (rt *Router) handleTrafficExportStatus(w http.ResponseWriter, _ *http.Request) {
	if rt.trafficExporter == nil {
		writeError(w, http.StatusNotImplemented, trafficExportNoKey)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, trafficExportStatusResponse{Sinks: rt.trafficExporter.Status()})
}
