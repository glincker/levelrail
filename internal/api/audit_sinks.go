package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/auditstream"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AuditSinkService is the surface the audit sink routes need from
// internal/auditstream; *auditstream.Service satisfies it.
type AuditSinkService interface {
	List(ctx context.Context) ([]store.AuditSink, error)
	Get(ctx context.Context, id string) (store.AuditSink, error)
	Create(ctx context.Context, in auditstream.Input) (store.AuditSink, string, error)
	Update(ctx context.Context, id string, p auditstream.Patch) (store.AuditSink, error)
	Delete(ctx context.Context, id string) error
	RotateSecret(ctx context.Context, id string) (string, error)
	Test(ctx context.Context, id string) error
	Deliveries(ctx context.Context, id string, limit int) ([]store.AuditSinkDelivery, error)
}

// SetAuditSinks enables the audit sink routes.
func (rt *Router) SetAuditSinks(s AuditSinkService) { rt.auditSinks = s }

func (rt *Router) auditSinksOrNotImplemented(w http.ResponseWriter) (AuditSinkService, bool) {
	if rt.auditSinks == nil {
		writeError(w, http.StatusNotImplemented, "audit log streaming is not configured on this control plane")
		return nil, false
	}
	return rt.auditSinks, true
}

type auditSinkConfigResource struct {
	Address   string `json:"address,omitempty"`
	TLS       bool   `json:"tls,omitempty"`
	URL       string `json:"url,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	Region    string `json:"region,omitempty"`
	Bucket    string `json:"bucket,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	PathStyle bool   `json:"path_style,omitempty"`
}

type auditSinkResource struct {
	ID                  string                  `json:"id"`
	Name                string                  `json:"name"`
	Kind                string                  `json:"kind"`
	Enabled             bool                    `json:"enabled"`
	Config              auditSinkConfigResource `json:"config"`
	CursorSeq           int64                   `json:"cursor_seq"`
	ConsecutiveFailures int                     `json:"consecutive_failures"`
	NextAttemptAt       string                  `json:"next_attempt_at,omitempty"`
	LastError           string                  `json:"last_error,omitempty"`
	LastDeliveredAt     string                  `json:"last_delivered_at,omitempty"`
	CreatedAt           string                  `json:"created_at"`
	UpdatedAt           string                  `json:"updated_at"`
	// SigningSecret is present only in the create and rotate responses.
	SigningSecret string `json:"signing_secret,omitempty"`
}

type auditSinkDeliveryResource struct {
	ID        string `json:"id"`
	SinkID    string `json:"sink_id"`
	FromSeq   int64  `json:"from_seq"`
	ToSeq     int64  `json:"to_seq"`
	Entries   int    `json:"entries"`
	State     string `json:"state"`
	Attempt   int    `json:"attempt"`
	LastError string `json:"last_error,omitempty"`
	CreatedAt string `json:"created_at"`
}

type auditSinkTestResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type createAuditSinkRequest struct {
	Name            string                  `json:"name"`
	Kind            string                  `json:"kind"`
	Enabled         *bool                   `json:"enabled,omitempty"`
	Config          auditSinkConfigResource `json:"config"`
	AccessKeyID     string                  `json:"access_key_id,omitempty"`
	SecretAccessKey string                  `json:"secret_access_key,omitempty"`
	Backfill        bool                    `json:"backfill,omitempty"`
}

type updateAuditSinkRequest struct {
	Name            *string                  `json:"name,omitempty"`
	Enabled         *bool                    `json:"enabled,omitempty"`
	Config          *auditSinkConfigResource `json:"config,omitempty"`
	AccessKeyID     *string                  `json:"access_key_id,omitempty"`
	SecretAccessKey *string                  `json:"secret_access_key,omitempty"`
}

func toStreamConfig(c auditSinkConfigResource) auditstream.SinkConfig {
	return auditstream.SinkConfig(c)
}

func toAuditSinkResource(k store.AuditSink) auditSinkResource {
	var cfg auditSinkConfigResource
	_ = json.Unmarshal([]byte(k.Config), &cfg)
	return auditSinkResource{
		ID: k.ID, Name: k.Name, Kind: k.Kind, Enabled: k.Enabled, Config: cfg, CursorSeq: k.CursorSeq,
		ConsecutiveFailures: k.ConsecutiveFailures, NextAttemptAt: k.NextAttemptAt, LastError: k.LastError,
		LastDeliveredAt: k.LastDeliveredAt, CreatedAt: k.CreatedAt, UpdatedAt: k.UpdatedAt,
	}
}

func (rt *Router) writeAuditSinkError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, auditstream.ErrInvalid):
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), auditstream.ErrInvalid.Error()+": "))
	case errors.Is(err, auditstream.ErrSecretsUnavailable):
		writeError(w, http.StatusConflict, "configure a master key before adding this sink, its credentials are stored encrypted")
	case errors.Is(err, store.ErrAuditSinkNotFound):
		writeError(w, http.StatusNotFound, "audit sink not found")
	case errors.Is(err, store.ErrAuditSinkNameTaken):
		writeError(w, http.StatusConflict, "an audit sink with this name already exists")
	default:
		rt.internalError(w, "api: "+op+" failed", err)
	}
}

func (rt *Router) handleListAuditSinks(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.auditSinksOrNotImplemented(w)
	if !ok {
		return
	}
	sinks, err := svc.List(r.Context())
	if err != nil {
		rt.writeAuditSinkError(w, "list audit sinks", err)
		return
	}
	out := make([]auditSinkResource, len(sinks))
	for i, k := range sinks {
		out[i] = toAuditSinkResource(k)
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleCreateAuditSink(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.auditSinksOrNotImplemented(w)
	if !ok {
		return
	}
	var req createAuditSinkRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	k, secret, err := svc.Create(r.Context(), auditstream.Input{
		Name: req.Name, Kind: req.Kind, Enabled: enabled, Config: toStreamConfig(req.Config),
		AccessKeyID: req.AccessKeyID, SecretAccessKey: req.SecretAccessKey, Backfill: req.Backfill,
	})
	if err != nil {
		rt.writeAuditSinkError(w, "create audit sink", err)
		return
	}
	res := toAuditSinkResource(k)
	res.SigningSecret = secret
	writeJSON(w, http.StatusCreated, res)
}

func (rt *Router) handleUpdateAuditSink(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.auditSinksOrNotImplemented(w)
	if !ok {
		return
	}
	var req updateAuditSinkRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p := auditstream.Patch{Name: req.Name, Enabled: req.Enabled, AccessKeyID: req.AccessKeyID, SecretAccessKey: req.SecretAccessKey}
	if req.Config != nil {
		c := toStreamConfig(*req.Config)
		p.Config = &c
	}
	k, err := svc.Update(r.Context(), r.PathValue("id"), p)
	if err != nil {
		rt.writeAuditSinkError(w, "update audit sink", err)
		return
	}
	writeJSON(w, http.StatusOK, toAuditSinkResource(k))
}

func (rt *Router) handleDeleteAuditSink(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.auditSinksOrNotImplemented(w)
	if !ok {
		return
	}
	if err := svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		rt.writeAuditSinkError(w, "delete audit sink", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) handleRotateAuditSinkSecret(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.auditSinksOrNotImplemented(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	secret, err := svc.RotateSecret(r.Context(), id)
	if err != nil {
		rt.writeAuditSinkError(w, "rotate audit sink secret", err)
		return
	}
	k, err := svc.Get(r.Context(), id)
	if err != nil {
		rt.writeAuditSinkError(w, "rotate audit sink secret", err)
		return
	}
	res := toAuditSinkResource(k)
	res.SigningSecret = secret
	writeJSON(w, http.StatusOK, res)
}

// handleTestAuditSink sends one probe. A delivery failure is the answer,
// not a server error, so it returns 200 with ok=false.
func (rt *Router) handleTestAuditSink(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.auditSinksOrNotImplemented(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if _, err := svc.Get(r.Context(), id); err != nil {
		rt.writeAuditSinkError(w, "test audit sink", err)
		return
	}
	if err := svc.Test(r.Context(), id); err != nil {
		msg := err.Error()
		if len(msg) > 300 {
			msg = msg[:300]
		}
		writeJSON(w, http.StatusOK, auditSinkTestResponse{OK: false, Error: msg})
		return
	}
	writeJSON(w, http.StatusOK, auditSinkTestResponse{OK: true})
}

func (rt *Router) handleListAuditSinkDeliveries(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.auditSinksOrNotImplemented(w)
	if !ok {
		return
	}
	limit := 50
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	ds, err := svc.Deliveries(r.Context(), r.PathValue("id"), limit)
	if err != nil {
		rt.writeAuditSinkError(w, "list audit sink deliveries", err)
		return
	}
	out := make([]auditSinkDeliveryResource, len(ds))
	for i, d := range ds {
		out[i] = auditSinkDeliveryResource{
			ID: d.ID, SinkID: d.SinkID, FromSeq: d.FromSeq, ToSeq: d.ToSeq, Entries: d.Entries,
			State: d.State, Attempt: d.Attempt, LastError: d.LastError, CreatedAt: d.CreatedAt,
		}
	}
	writeJSON(w, http.StatusOK, out)
}
