package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/mailbackup"
	"github.com/GLINCKER/levelrail/internal/mailstorage"
)

const (
	mailStorageLocal  = "local"
	mailStorageBucket = "s3"
)

// mailStorageRecord is the saved destination. The bucket secret is sealed in
// the secrets store under mailstorage.SecretSlot, never in this file.
type mailStorageRecord struct {
	Kind       string                  `json:"kind"`
	Connection *mailstorage.Connection `json:"connection,omitempty"`
	Steps      []mailstorage.Step      `json:"steps,omitempty"`
	UpdatedAt  time.Time               `json:"updated_at"`
}

type mailStorageBucketView struct {
	Endpoint  string `json:"endpoint,omitempty"`
	Region    string `json:"region,omitempty"`
	Bucket    string `json:"bucket"`
	PathStyle bool   `json:"path_style"`
}

type mailStorageResponse struct {
	App    string                 `json:"app"`
	Kind   string                 `json:"kind"`
	Folder string                 `json:"folder"`
	Cap    int64                  `json:"cap_bytes"`
	Bucket *mailStorageBucketView `json:"bucket,omitempty"`
	Steps  []mailstorage.Step     `json:"steps"`
	OK     bool                   `json:"ok"`
	Saved  bool                   `json:"saved"`
	// BucketUploads is false in this release: backups stay in the local folder.
	BucketUploads bool `json:"bucket_uploads"`
}

type mailStorageRequest struct {
	App         string `json:"app"`
	Kind        string `json:"kind"`
	Endpoint    string `json:"endpoint"`
	Region      string `json:"region"`
	Bucket      string `json:"bucket"`
	AccessKeyID string `json:"access_key_id"`
	// SecretAccessKey is write-only: it is sealed and never returned.
	SecretAccessKey string `json:"secret_access_key"` //nolint:gosec // write-only request field
	PathStyle       bool   `json:"path_style"`
}

func (c *mailBackupConfig) recordPath(app string) string {
	return filepath.Join(c.localDir(app), ".destination.json")
}

func (c *mailBackupConfig) loadRecord(app string) mailStorageRecord {
	rec := mailStorageRecord{Kind: mailStorageLocal}
	raw, err := os.ReadFile(c.recordPath(app)) //nolint:gosec // app is validated by mailstorage.ValidServiceID
	if err == nil {
		_ = json.Unmarshal(raw, &rec)
	}
	if rec.Kind != mailStorageBucket || rec.Connection == nil {
		rec.Kind, rec.Connection = mailStorageLocal, nil
	}
	return rec
}

func (c *mailBackupConfig) saveRecord(app string, rec mailStorageRecord) error {
	if err := os.MkdirAll(c.localDir(app), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	tmp := c.recordPath(app) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.recordPath(app))
}

func (c *mailBackupConfig) view(app string, rec mailStorageRecord) mailStorageResponse {
	capB, _ := mailbackup.CapFromEnv(os.Getenv)
	out := mailStorageResponse{App: app, Kind: rec.Kind, Folder: c.localDir(app), Cap: capB, Steps: rec.Steps, OK: true}
	if out.Steps == nil {
		out.Steps = []mailstorage.Step{}
	}
	if rec.Connection != nil {
		out.Bucket = &mailStorageBucketView{Endpoint: rec.Connection.Endpoint, Region: rec.Connection.Region, Bucket: rec.Connection.Bucket, PathStyle: rec.Connection.PathStyle}
	}
	for _, s := range out.Steps {
		out.OK = out.OK && s.OK
	}
	return out
}

// handleGetMailStorage handles GET /api/v1/mail/storage.
func (rt *Router) handleGetMailStorage(w http.ResponseWriter, r *http.Request) {
	cfg, t, ok := rt.mailBackupSetup(w, r, r.URL.Query().Get("app"), false)
	if !ok {
		return
	}
	out := cfg.view(t.svc.Name, cfg.loadRecord(t.svc.Name))
	out.Saved = true
	writeJSON(w, http.StatusOK, out)
}

// handlePutMailStorage handles PUT /api/v1/mail/storage. A bucket is saved
// only after the put, get, list and delete probe passes.
func (rt *Router) handlePutMailStorage(w http.ResponseWriter, r *http.Request) {
	var req mailStorageRequest
	if err := decodeJSONBodyLimit(r, &req, 1<<14); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cfg, t, ok := rt.mailBackupSetup(w, r, req.App, false)
	if !ok {
		return
	}
	app := t.svc.Name
	prev := cfg.loadRecord(app)
	switch strings.TrimSpace(req.Kind) {
	case mailStorageLocal, "":
		if prev.Connection != nil {
			_ = mailstorage.Load(cfg.secrets, *prev.Connection).Forget(r.Context())
		}
		rec := mailStorageRecord{Kind: mailStorageLocal, UpdatedAt: time.Now().UTC()}
		if err := cfg.saveRecord(app, rec); err != nil {
			rt.internalError(w, "api: mail storage: save", err)
			return
		}
		out := cfg.view(app, rec)
		out.Saved = true
		writeJSON(w, http.StatusOK, out)
		return
	case mailStorageBucket:
	default:
		writeError(w, http.StatusBadRequest, "kind must be local or s3")
		return
	}
	if cfg.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secret storage is not configured, a bucket cannot be saved")
		return
	}
	spec := mailstorage.Spec{
		ServiceID: app, Endpoint: req.Endpoint, Region: req.Region, Bucket: req.Bucket,
		AccessKeyID: req.AccessKeyID, Secret: req.SecretAccessKey, PathStyle: req.PathStyle,
	}
	cand, err := mailstorage.Candidate(spec)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Probe before sealing: the secret slot is shared, so a failed candidate
	// must leave the working configuration exactly as it was.
	res := mailstorage.Probe(r.Context(), cand)
	if !res.OK {
		out := cfg.view(app, mailStorageRecord{Kind: mailStorageBucket, Connection: &cand, Steps: res.Steps, UpdatedAt: time.Now().UTC()})
		out.OK, out.Saved = false, false
		writeJSON(w, http.StatusUnprocessableEntity, out)
		return
	}
	conn, err := mailstorage.New(r.Context(), cfg.secrets, spec)
	if err != nil {
		rt.internalError(w, "api: mail storage: seal", err)
		return
	}
	rec := mailStorageRecord{Kind: mailStorageBucket, Connection: &conn, Steps: res.Steps, UpdatedAt: time.Now().UTC()}
	if err := cfg.saveRecord(app, rec); err != nil {
		rt.internalError(w, "api: mail storage: save", err)
		return
	}
	out := cfg.view(app, rec)
	out.Saved = true
	writeJSON(w, http.StatusOK, out)
}

// handleTestMailStorage handles POST /api/v1/mail/storage/test: it probes the
// saved bucket again, or checks that the local folder is writable.
func (rt *Router) handleTestMailStorage(w http.ResponseWriter, r *http.Request) {
	var req mailBackupRunRequest
	if err := decodeJSONBodyLimit(r, &req, 1<<12); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cfg, t, ok := rt.mailBackupSetup(w, r, req.App, false)
	if !ok {
		return
	}
	app := t.svc.Name
	rec := cfg.loadRecord(app)
	if rec.Connection == nil {
		step := mailstorage.Step{Name: "write", OK: true}
		if err := cfg.saveRecord(app, rec); err != nil {
			step = mailstorage.Step{Name: "write", Error: "the local backup folder is not writable"}
		}
		rec.Steps = []mailstorage.Step{step}
	} else {
		res := mailstorage.Probe(r.Context(), mailstorage.Load(cfg.secrets, *rec.Connection))
		rec.Steps = res.Steps
		_ = cfg.saveRecord(app, rec)
	}
	out := cfg.view(app, rec)
	out.Saved = true
	writeJSON(w, http.StatusOK, out)
}
