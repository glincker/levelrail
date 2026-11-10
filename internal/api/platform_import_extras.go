package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/platformimport/dokployextras"
)

const importInprocHost = "http://import.internal"

// platformImportResponse is the import report plus the backup and alert channel plan.
// Neither half has a key, token or webhook URL field, only names, shapes and has_* flags.
type platformImportResponse struct {
	platformimport.Report
	Backups  *dokployextras.BackupImport  `json:"backups,omitempty"`
	Channels *dokployextras.ChannelImport `json:"channels,omitempty"`
	// Journal lists what this run created, by id and fingerprint, for resume and undo.
	Journal []dokployextras.Record `json:"journal,omitempty"`
}

// importInproc holds the full handler chain, so imported backups and channels are created
// through the normal create routes with the caller's own credentials, scoping and audit.
type importInproc struct {
	rt    *Router
	inner http.Handler
}

// importExtras plans backups and channels from a discovery; nil halves mean the source has none.
func importExtras(ctx context.Context, req platformImportRequest, disc *platformimport.Discovery) (*dokployextras.BackupImport, *dokployextras.ChannelImport) {
	if disc == nil {
		return nil, nil
	}
	// The request was already checked by platformImportPolicy, so this cannot fail here.
	policy, _ := platformImportPolicy(req)
	backups := dokployextras.PlanBackups(disc, dokployextras.NewBackupOptions(ctx, req.EnableBackupSchedules, policy, os.LookupEnv))
	backups.FinishPlan(req.IncludeCredentials)
	channels := dokployextras.PlanChannels(disc, dokployextras.NewChannelOptions(ctx, req.EnableImportedChannels, policy, os.LookupEnv))
	channels.FinishPlan(req.IncludeCredentials)
	return backups, channels
}

func (req platformImportRequest) wantsExtras() bool {
	return req.ImportExtras || req.ExtrasOnly || req.IncludeCredentials || req.EnableBackupSchedules || req.EnableImportedChannels
}

// undoRefRE bounds journal ids and names to what the create routes return, so neither can
// steer the in-process request to another path.
var undoRefRE = regexp.MustCompile(`^[A-Za-z0-9_:-]{1,128}$`)

type destinationCredentialer interface {
	DestinationCredentials(ctx context.Context, sourceID string) (string, string, error)
}

type notificationTargeter interface {
	NotificationTarget(ctx context.Context, sourceID string) (platformimport.NotifyTarget, error)
}

// applyImportExtras creates the planned targets, schedules and channels. Source keys and
// notify URLs are read only here, only with include_credentials, and never returned.
func (ip *importInproc) applyImportExtras(r *http.Request, req platformImportRequest, src platformimport.Source, resp *platformImportResponse) {
	// Extras are created only on an explicit opt-in, so a plain apply never adds backups or channels.
	if (resp.Backups == nil && resp.Channels == nil) || !req.wantsExtras() {
		return
	}
	if req.IncludeCredentials {
		if c, ok := src.(destinationCredentialer); ok && resp.Backups != nil {
			resp.Backups.Creds = c.DestinationCredentials
		}
		if t, ok := src.(notificationTargeter); ok && resp.Channels != nil {
			resp.Channels.Target = t.NotificationTarget
		}
	}
	client := ip.client(r)
	jr := &importJournal{prior: req.Journal}
	dokployextras.ApplyBackups(r.Context(), client, resp.Backups, jr)
	dokployextras.ApplyChannels(r.Context(), client, resp.Channels, jr)
	resp.Journal = jr.out
}

func (ip *importInproc) client(r *http.Request) *apiclient.Client {
	return apiclient.NewClient(importInprocHost, "", apiclient.WithTransport(&importInprocTransport{inner: ip.inner, origin: r,
		maxResponse: envInt64Positive(envMCPHTTPMaxResponse, mcpHTTPDefaultResponse),
		timeout:     time.Duration(envPositiveInt(envMCPHTTPCallTimeout, mcpHTTPDefaultTimeout)) * time.Second}))
}

// importJournal keeps the records of this run; prior holds a previous run's records for resume.
type importJournal struct {
	prior []dokployextras.Record
	out   []dokployextras.Record
}

func (j *importJournal) FindCreated(kind, name string) (dokployextras.Record, bool) {
	for _, r := range j.prior {
		if r.Kind == kind && r.Name == name && r.ID != "" {
			return r, true
		}
	}
	return dokployextras.Record{}, false
}

func (j *importJournal) Created(r dokployextras.Record) { j.out = append(j.out, r) }
func (j *importJournal) Resumed(r dokployextras.Record) { j.out = append(j.out, r) }
func (j *importJournal) Failed(string, string)          {}

// importInprocTransport replays a call on the full handler chain as the original caller:
// same cookie or bearer token, same origin, so authz, tenant scope and CSRF apply unchanged.
type importInprocTransport struct {
	inner       http.Handler
	origin      *http.Request
	maxResponse int64
	timeout     time.Duration
}

var importInprocSkipHeaders = map[string]bool{"Content-Length": true, "Content-Type": true, "Content-Encoding": true,
	"Accept-Encoding": true, "Transfer-Encoding": true, "Idempotency-Key": true, "Accept": true}

func (t *importInprocTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.inner == nil {
		return nil, errors.New("the in-process API is not ready")
	}
	ctx, cancel := context.WithTimeout(req.Context(), t.timeout)
	defer cancel()
	in := req.Clone(ctx)
	in.URL = &url.URL{Path: req.URL.Path, RawPath: req.URL.RawPath, RawQuery: req.URL.RawQuery}
	in.RequestURI = in.URL.RequestURI()
	in.Host = t.origin.Host
	in.RemoteAddr = t.origin.RemoteAddr
	in.TLS = t.origin.TLS
	for k, vs := range t.origin.Header {
		if importInprocSkipHeaders[http.CanonicalHeaderKey(k)] || in.Header.Get(k) != "" {
			continue
		}
		in.Header[k] = vs
	}
	rec := &mcpBufferedResponse{header: http.Header{}, status: http.StatusOK, limit: t.maxResponse}
	t.inner.ServeHTTP(rec, in)
	if rec.overflow {
		return nil, errors.New("in-process API response exceeded the size limit")
	}
	return &http.Response{
		Status: http.StatusText(rec.status), StatusCode: rec.status, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: rec.header, Body: io.NopCloser(bytes.NewReader(rec.buf.Bytes())),
		ContentLength: int64(rec.buf.Len()), Request: req,
	}, nil
}

type platformImportUndoRequest struct {
	Journal []dokployextras.Record `json:"journal"`
}

type platformImportUndoItem struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	ID     string `json:"id,omitempty"`
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

type platformImportUndoResponse struct {
	Items []platformImportUndoItem `json:"items"`
}

// handleUndo handles POST /api/v1/imports/platform/undo. It removes a journaled target,
// schedule or channel only when it reads back by id with the recorded fingerprint, and
// removes it through the normal delete route, so a forged record gains nothing.
func (ip *importInproc) handleUndo(w http.ResponseWriter, r *http.Request) {
	var req platformImportUndoRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPlatformImportBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	limit := dokployextras.EnvPositiveInt(os.LookupEnv, dokployextras.EnvLimit, dokployextras.DefaultLimit, dokployextras.CeilingLimit)
	if len(req.Journal) > limit {
		writeError(w, http.StatusBadRequest, "too many journal entries ("+dokployextras.EnvLimit+")")
		return
	}
	for _, rec := range req.Journal {
		if !undoRefRE.MatchString(rec.Name) || (rec.ID != "" && !undoRefRE.MatchString(rec.ID)) {
			writeError(w, http.StatusBadRequest, "journal entries carry an invalid name or id")
			return
		}
	}
	timeout := time.Duration(envPositiveInt(envMCPHTTPCallTimeout, mcpHTTPDefaultTimeout)) * time.Second
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	client := ip.client(r)
	channels, listErr := client.ListNotificationChannels(ctx)
	resp := platformImportUndoResponse{Items: []platformImportUndoItem{}}
	// Schedules first, then channels, then targets, so a target is never removed under its schedule.
	for _, kind := range []string{dokployextras.KindBackupSchedule, dokployextras.KindChannel, dokployextras.KindBackupTarget} {
		for _, rec := range req.Journal {
			if rec.Kind != kind {
				continue
			}
			item := platformImportUndoItem{Kind: rec.Kind, Name: dokployextras.CleanDetail(rec.Name), ID: dokployextras.CleanDetail(rec.ID), Action: "skipped"}
			var remove bool
			var reason string
			if rec.Kind == dokployextras.KindChannel {
				remove, reason = dokployextras.PlanChannelUndo(channels, listErr, rec)
			} else {
				remove, reason = dokployextras.PlanUndo(ctx, client, rec)
			}
			switch {
			case !remove:
				item.Reason = reason
			case dokployextras.Undo(ctx, client, rec) != nil:
				item.Action, item.Reason = "failed", "the delete call failed; check it by hand"
			default:
				item.Action = "removed"
			}
			resp.Items = append(resp.Items, item)
		}
	}
	ip.rt.logger.Info("api: platform import undo", slog.Int("entries", len(req.Journal)))
	writeJSON(w, http.StatusOK, resp)
}
