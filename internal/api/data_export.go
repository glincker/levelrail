package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dataexport"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DataExportService is the surface the export routes need from
// internal/dataexport; *dataexport.Service satisfies it.
type DataExportService interface {
	List(ctx context.Context) ([]store.ExportDestination, error)
	Get(ctx context.Context, id string) (store.ExportDestination, error)
	Create(ctx context.Context, in dataexport.Input) (store.ExportDestination, error)
	Update(ctx context.Context, id string, p dataexport.Patch) (store.ExportDestination, error)
	Delete(ctx context.Context, id string) error
	Test(ctx context.Context, id string) error
	Status(ctx context.Context, id string) (store.ExportStatus, error)
	Series(ctx context.Context, allow dataexport.ResourceFilter) ([]dataexport.Series, error)
	StreamAudit(ctx context.Context, from, to time.Time, fn func(dataexport.AuditRecord) error) error
}

// SetDataExport enables the data export routes and the /metrics scrape
// endpoint after the router is built.
func (rt *Router) SetDataExport(s DataExportService) { rt.dataExport = s }

func (rt *Router) dataExportOrNotImplemented(w http.ResponseWriter) (DataExportService, bool) {
	if rt.dataExport == nil {
		writeError(w, http.StatusNotImplemented, "data export is not configured on this control plane")
		return nil, false
	}
	return rt.dataExport, true
}

type exportStatusResource struct {
	LastSuccessAt string `json:"last_success_at"`
	LastError     string `json:"last_error"`
	LastErrorAt   string `json:"last_error_at"`
	SentTotal     int64  `json:"sent_total"`
	DroppedTotal  int64  `json:"dropped_total"`
	FailuresTotal int64  `json:"failures_total"`
	QueueDepth    int64  `json:"queue_depth"`
	CircuitOpen   bool   `json:"circuit_open"`
	CursorAt      string `json:"cursor_at,omitempty"`
}

type exportOptionsResource struct {
	StorageTargetID string            `json:"storage_target_id,omitempty"`
	Prefix          string            `json:"prefix,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	IntervalSeconds int               `json:"interval_seconds,omitempty"`
	Backfill        bool              `json:"backfill,omitempty"`
	Region          string            `json:"region,omitempty"`
	Site            string            `json:"site,omitempty"`
	Dataset         string            `json:"dataset,omitempty"`
	Username        string            `json:"username,omitempty"`
	// Audit stream sink settings (audit_splunk, audit_syslog, audit_otlp).
	Index              string `json:"index,omitempty"`
	Source             string `json:"source,omitempty"`
	SourceType         string `json:"source_type,omitempty"`
	Gzip               bool   `json:"gzip,omitempty"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify,omitempty"`
	Facility           string `json:"facility,omitempty"`
	AppName            string `json:"app_name,omitempty"`
}

// exportDestinationResource never carries header values or the signing
// key: only the header names and whether a key is set.
type exportDestinationResource struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Kind        store.ExportKind      `json:"kind"`
	Endpoint    string                `json:"endpoint"`
	Enabled     bool                  `json:"enabled"`
	Apps        []string              `json:"apps"`
	MinLevel    string                `json:"min_level"`
	Options     exportOptionsResource `json:"options"`
	HeaderNames []string              `json:"header_names"`
	HasHMACKey  bool                  `json:"has_hmac_key"`
	// HasAPIKey is true for a vendor destination that has its key stored.
	HasAPIKey bool                 `json:"has_api_key"`
	Status    exportStatusResource `json:"status"`
	CreatedAt string               `json:"created_at"`
	UpdatedAt string               `json:"updated_at"`
}

func toExportOptions(o store.ExportOptions) exportOptionsResource {
	return exportOptionsResource{StorageTargetID: o.StorageTargetID, Prefix: o.Prefix, Labels: o.Labels, IntervalSeconds: o.IntervalSeconds, Backfill: o.Backfill, Region: o.Region, Site: o.Site, Dataset: o.Dataset, Username: o.Username,
		Index: o.Index, Source: o.Source, SourceType: o.SourceType, Gzip: o.Gzip, InsecureSkipVerify: o.InsecureSkipVerify, Facility: o.Facility, AppName: o.AppName}
}

func (o exportOptionsResource) toStore() store.ExportOptions {
	return store.ExportOptions{StorageTargetID: o.StorageTargetID, Prefix: o.Prefix, Labels: o.Labels, IntervalSeconds: o.IntervalSeconds, Backfill: o.Backfill, Region: o.Region, Site: o.Site, Dataset: o.Dataset, Username: o.Username,
		Index: o.Index, Source: o.Source, SourceType: o.SourceType, Gzip: o.Gzip, InsecureSkipVerify: o.InsecureSkipVerify, Facility: o.Facility, AppName: o.AppName}
}

func toExportDestinationResource(d store.ExportDestination, st store.ExportStatus) exportDestinationResource {
	apps, names := d.Apps, d.Options.HeaderNames
	if apps == nil {
		apps = []string{}
	}
	if names == nil {
		names = []string{}
	}
	return exportDestinationResource{
		ID: d.ID, Name: d.Name, Kind: d.Kind, Endpoint: d.Endpoint, Enabled: d.Enabled, Apps: apps, MinLevel: d.MinLevel,
		Options: toExportOptions(d.Options), HeaderNames: names, HasHMACKey: d.Options.HasHMACKey,
		HasAPIKey: (dataexport.IsVendorLogKind(d.Kind) || d.Kind == store.ExportKindMetricsRemoteWrite || d.Kind == store.ExportKindAuditSplunk) && len(names) > 0,
		Status: exportStatusResource{
			LastSuccessAt: st.LastSuccessAt, LastError: st.LastError, LastErrorAt: st.LastErrorAt, SentTotal: st.SentTotal,
			DroppedTotal: st.DroppedTotal, FailuresTotal: st.FailuresTotal, QueueDepth: st.QueueDepth,
			CircuitOpen: st.CircuitOpen, CursorAt: st.CursorAt,
		},
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

type createExportDestinationRequest struct {
	Name     string                `json:"name"`
	Kind     store.ExportKind      `json:"kind"`
	Endpoint string                `json:"endpoint"`
	Enabled  *bool                 `json:"enabled"`
	Apps     []string              `json:"apps"`
	MinLevel string                `json:"min_level"`
	Options  exportOptionsResource `json:"options"`
	Headers  map[string]string     `json:"headers"`
	HMACKey  string                `json:"hmac_key"`
	APIKey   string                `json:"api_key"`
}

type updateExportDestinationRequest struct {
	Name     *string                `json:"name"`
	Endpoint *string                `json:"endpoint"`
	Enabled  *bool                  `json:"enabled"`
	Apps     *[]string              `json:"apps"`
	MinLevel *string                `json:"min_level"`
	Options  *exportOptionsResource `json:"options"`
	Headers  *map[string]string     `json:"headers"`
	HMACKey  *string                `json:"hmac_key"`
	APIKey   *string                `json:"api_key"`
}

func (rt *Router) writeExportError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, dataexport.ErrInvalid):
		msg := strings.TrimPrefix(err.Error(), dataexport.ErrInvalid.Error()+": ")
		writeError(w, http.StatusBadRequest, msg)
	case errors.Is(err, store.ErrExportDestinationNotFound):
		writeError(w, http.StatusNotFound, "export destination not found")
	case errors.Is(err, store.ErrExportDestinationNameTaken):
		writeError(w, http.StatusConflict, "an export destination with this name already exists")
	default:
		rt.internalError(w, "api: "+op+" failed", err)
	}
}

func (rt *Router) exportResource(r *http.Request, svc DataExportService, d store.ExportDestination) exportDestinationResource {
	st, err := svc.Status(r.Context(), d.ID)
	if err != nil {
		st = store.ExportStatus{DestinationID: d.ID}
	}
	return toExportDestinationResource(d, st)
}

func (rt *Router) handleListExportDestinations(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.dataExportOrNotImplemented(w)
	if !ok {
		return
	}
	dests, err := svc.List(r.Context())
	if err != nil {
		rt.writeExportError(w, "list export destinations", err)
		return
	}
	out := make([]exportDestinationResource, len(dests))
	for i, d := range dests {
		out[i] = rt.exportResource(r, svc, d)
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleGetExportDestination(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.dataExportOrNotImplemented(w)
	if !ok {
		return
	}
	d, err := svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.writeExportError(w, "get export destination", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.exportResource(r, svc, d))
}

func (rt *Router) handleCreateExportDestination(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.dataExportOrNotImplemented(w)
	if !ok {
		return
	}
	var req createExportDestinationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusBadRequest, ""), appRefs(req.Apps)...) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	d, err := svc.Create(r.Context(), dataexport.Input{
		Name: req.Name, Kind: req.Kind, Endpoint: req.Endpoint, Enabled: enabled, Apps: req.Apps,
		MinLevel: req.MinLevel, Options: req.Options.toStore(), Headers: req.Headers, HMACKey: req.HMACKey, APIKey: req.APIKey,
	})
	if err != nil {
		rt.writeExportError(w, "create export destination", err)
		return
	}
	writeJSON(w, http.StatusCreated, rt.exportResource(r, svc, d))
}

func (rt *Router) handleUpdateExportDestination(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.dataExportOrNotImplemented(w)
	if !ok {
		return
	}
	var req updateExportDestinationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Apps != nil && !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusBadRequest, ""), appRefs(*req.Apps)...) {
		return
	}
	p := dataexport.Patch{
		Name: req.Name, Endpoint: req.Endpoint, Enabled: req.Enabled, Apps: req.Apps, MinLevel: req.MinLevel,
		Headers: req.Headers, HMACKey: req.HMACKey, APIKey: req.APIKey,
	}
	if req.Options != nil {
		o := req.Options.toStore()
		p.Options = &o
	}
	d, err := svc.Update(r.Context(), r.PathValue("id"), p)
	if err != nil {
		rt.writeExportError(w, "update export destination", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.exportResource(r, svc, d))
}

func (rt *Router) handleDeleteExportDestination(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.dataExportOrNotImplemented(w)
	if !ok {
		return
	}
	if err := svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		rt.writeExportError(w, "delete export destination", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type exportTestResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	// Fix names the remedy the dashboard offers: bad_token, tls_error, unreachable, rejected or private_blocked.
	Fix string `json:"fix,omitempty"`
}

// handleTestExportDestination sends one probe. A delivery failure is the
// answer, not a server error, so it returns 200 with ok=false.
func (rt *Router) handleTestExportDestination(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.dataExportOrNotImplemented(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if _, err := svc.Get(r.Context(), id); err != nil {
		rt.writeExportError(w, "test export destination", err)
		return
	}
	if err := svc.Test(r.Context(), id); err != nil {
		msg := err.Error()
		if len(msg) > 300 {
			msg = msg[:300]
		}
		writeJSON(w, http.StatusOK, exportTestResponse{OK: false, Error: msg, Fix: string(dataexport.ClassifyTestError(err))})
		return
	}
	writeJSON(w, http.StatusOK, exportTestResponse{OK: true})
}
