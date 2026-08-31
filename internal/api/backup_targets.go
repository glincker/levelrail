package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// backupTargetTestTimeout bounds the synchronous connectivity check
// handleTestBackupTarget runs, the same shape
// registryCredentialTestTimeout bounds its own synchronous test.
const backupTargetTestTimeout = 10 * time.Second

// BackupTargetTester is the surface the test-connection handler needs
// from internal/backup.TargetTester: probe one target's configured
// bucket over its stored credentials, without uploading or deleting
// anything. *backup.TargetTester satisfies this structurally;
// internal/api never resolves a backup target's credentials directly,
// the same boundary BackupSecretsSetter's own doc comment describes.
type BackupTargetTester interface {
	TestTarget(ctx context.Context, targetID string) error
}

// BackupTargetStore is the store surface the backup target handlers
// need.
type BackupTargetStore interface {
	SaveBackupTarget(ctx context.Context, t store.BackupTarget) error
	GetBackupTarget(ctx context.Context, id string) (store.BackupTarget, error)
	ListBackupTargets(ctx context.Context) ([]store.BackupTarget, error)
	UpdateBackupTarget(ctx context.Context, id, name, provider, endpoint, region, bucket string) error
	DeleteBackupTarget(ctx context.Context, id string) error
}

// BackupSecretsSetter is the surface backup target creation needs from
// internal/secrets.Manager: store a credential value, the same "set a
// value, never read one back through this interface" shape SecretSetter
// already establishes for app secrets. A distinct type from SecretSetter
// even though both are structurally satisfied by *secrets.Manager,
// because a backup target's credentials do get resolved back out
// server-side, just not through this interface or this package: that's
// internal/backup.Runner's own, separate, narrower
// internal/backup.SecretsResolver, wired directly in cmd/levelrail, not
// through the Router at all.
type BackupSecretsSetter interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
}

// backupTargetResource is the wire shape for a connected backup
// destination. Deliberately no credential fields, request or response:
// AccessKeyID/SecretAccessKey are accepted only through
// createBackupTargetRequest below and never echoed back, the same
// "a secret's value and a resource's other fields have different write
// (and read) paths" reasoning handleSetSecret's own doc comment
// establishes for app secrets.
type backupTargetResource struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	Endpoint  string `json:"endpoint,omitempty"`
	Region    string `json:"region,omitempty"`
	Bucket    string `json:"bucket"`
	CreatedAt string `json:"created_at"`
}

func toBackupTargetResource(t store.BackupTarget) backupTargetResource {
	return backupTargetResource{
		ID:        t.ID,
		Name:      t.Name,
		Provider:  t.Provider,
		Endpoint:  t.Endpoint,
		Region:    t.Region,
		Bucket:    t.Bucket,
		CreatedAt: t.CreatedAt,
	}
}

// createBackupTargetRequest is createBackupTargetResource's request
// body: backupTargetResource's fields plus the two write-only credential
// fields no response type ever carries.
type createBackupTargetRequest struct {
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	Endpoint        string `json:"endpoint,omitempty"`
	Region          string `json:"region,omitempty"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
}

// validateCreateBackupTargetRequest checks everything except whether the
// credentials actually work: this endpoint saves what it's given, the
// same "always succeeds at the store layer, the reconciler is what
// actually proves it works" boundary handleCreateDatabase's own doc
// comment describes for a database engine with no secrets wired up yet.
// Proving a bucket and its credentials are real is exactly what the
// first real backup attempt does; duplicating that check here (a live
// S3 round trip on every create call) would make this endpoint's latency
// and failure modes depend on network access to a third-party service
// it has no other reason to need.
// validateBackupTargetFields checks name/provider/endpoint/bucket, the
// fields createBackupTargetRequest and updateBackupTargetRequest validate
// identically; each then layers its own credential-field rules on top.
func validateBackupTargetFields(name, provider, endpoint, bucket string) error {
	if name == "" {
		return errors.New("name is required")
	}
	switch provider {
	case store.BackupProviderAWS, store.BackupProviderR2, store.BackupProviderCustom:
	default:
		return fmt.Errorf("provider must be one of %q, %q, %q", store.BackupProviderAWS, store.BackupProviderR2, store.BackupProviderCustom)
	}
	if bucket == "" {
		return errors.New("bucket is required")
	}
	if endpoint == "" && provider != store.BackupProviderAWS {
		return errors.New("endpoint is required for r2 and custom providers")
	}
	return nil
}

func validateCreateBackupTargetRequest(req createBackupTargetRequest) error {
	if err := validateBackupTargetFields(req.Name, req.Provider, req.Endpoint, req.Bucket); err != nil {
		return err
	}
	if req.AccessKeyID == "" {
		return errors.New("access_key_id is required")
	}
	if req.SecretAccessKey == "" {
		return errors.New("secret_access_key is required")
	}
	return nil
}

// updateBackupTargetRequest is handleUpdateBackupTarget's request body:
// the same shape createBackupTargetRequest uses, except AccessKeyID and
// SecretAccessKey are optional. Left blank, the target's stored
// credentials are kept unchanged; set together, they rotate the stored
// credentials in place, so a leaked key doesn't require a delete and
// recreate to fix.
type updateBackupTargetRequest struct {
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	Endpoint        string `json:"endpoint,omitempty"`
	Region          string `json:"region,omitempty"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
}

func validateUpdateBackupTargetRequest(req updateBackupTargetRequest) error {
	if err := validateBackupTargetFields(req.Name, req.Provider, req.Endpoint, req.Bucket); err != nil {
		return err
	}
	if (req.AccessKeyID == "") != (req.SecretAccessKey == "") {
		return errors.New("access_key_id and secret_access_key must be set together to rotate credentials")
	}
	return nil
}

// handleListBackupTargets handles GET /api/v1/backup-targets.
func (rt *Router) handleListBackupTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := rt.backupTargets.ListBackupTargets(r.Context())
	if err != nil {
		rt.logger.Error("api: list backup targets failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]backupTargetResource, 0, len(targets))
	for _, t := range targets {
		out = append(out, toBackupTargetResource(t))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetBackupTarget handles GET /api/v1/backup-targets/{id}.
func (rt *Router) handleGetBackupTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	t, err := rt.backupTargets.GetBackupTarget(r.Context(), id)
	if errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusNotFound, "backup target not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get backup target failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toBackupTargetResource(t))
}

// handleCreateBackupTarget handles POST /api/v1/backup-targets.
// AbilityWriteSensitive (router.go), the same ability
// PUT /apps/{name}/secrets/{key} requires: this endpoint accepts live
// bucket credentials in its request body, not just ordinary resource
// fields.
//
// Credentials are written to internal/secrets before the store row, not
// after: if the store write then fails, the result is an orphaned,
// unreferenced secret (harmless, nothing ever looks it up without a
// matching backup_targets row) rather than a backup target that exists
// and looks connected but has no working credentials behind it, which
// is the failure this ordering is chosen specifically to avoid.
func (rt *Router) handleCreateBackupTarget(w http.ResponseWriter, r *http.Request) {
	if rt.backupSecrets == nil {
		writeError(w, http.StatusNotImplemented, "backup targets are not configured on this control plane (no master key set)")
		return
	}

	var req createBackupTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateCreateBackupTargetRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := randomBackupTargetID()
	if err != nil {
		rt.logger.Error("api: create backup target: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	secretsKey := store.BackupTargetSecretsKey(id)
	if err := rt.backupSecrets.SetValue(r.Context(), secretsKey, "access_key_id", req.AccessKeyID); err != nil {
		rt.logger.Error("api: create backup target: set access_key_id failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := rt.backupSecrets.SetValue(r.Context(), secretsKey, "secret_access_key", req.SecretAccessKey); err != nil {
		rt.logger.Error("api: create backup target: set secret_access_key failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	target := store.BackupTarget{
		ID:        id,
		Name:      req.Name,
		Provider:  req.Provider,
		Endpoint:  req.Endpoint,
		Region:    req.Region,
		Bucket:    req.Bucket,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := rt.backupTargets.SaveBackupTarget(r.Context(), target); err != nil {
		rt.logger.Error("api: create backup target: save failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toBackupTargetResource(target))
}

// handleUpdateBackupTarget handles PUT /api/v1/backup-targets/{id}:
// AbilityWriteSensitive, the same tier as create and delete. A full
// replace of name/provider/endpoint/region/bucket; credentials rotate only
// when both access_key_id and secret_access_key are present in the
// request, the same "written before the store row" ordering
// handleCreateBackupTarget's own doc comment establishes.
func (rt *Router) handleUpdateBackupTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, err := rt.backupTargets.GetBackupTarget(r.Context(), id); errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusNotFound, "backup target not found")
		return
	} else if err != nil {
		rt.logger.Error("api: update backup target: load failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req updateBackupTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateUpdateBackupTargetRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.AccessKeyID != "" {
		if rt.backupSecrets == nil {
			writeError(w, http.StatusNotImplemented, "backup targets are not configured on this control plane (no master key set)")
			return
		}
		secretsKey := store.BackupTargetSecretsKey(id)
		if err := rt.backupSecrets.SetValue(r.Context(), secretsKey, "access_key_id", req.AccessKeyID); err != nil {
			rt.logger.Error("api: update backup target: set access_key_id failed", slog.String("error", err.Error()), slog.String("id", id))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := rt.backupSecrets.SetValue(r.Context(), secretsKey, "secret_access_key", req.SecretAccessKey); err != nil {
			rt.logger.Error("api: update backup target: set secret_access_key failed", slog.String("error", err.Error()), slog.String("id", id))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if err := rt.backupTargets.UpdateBackupTarget(r.Context(), id, req.Name, req.Provider, req.Endpoint, req.Region, req.Bucket); err != nil {
		if errors.Is(err, store.ErrBackupTargetNotFound) {
			writeError(w, http.StatusNotFound, "backup target not found")
			return
		}
		rt.logger.Error("api: update backup target failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	updated, err := rt.backupTargets.GetBackupTarget(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: update backup target: reload after update failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toBackupTargetResource(updated))
}

// handleDeleteBackupTarget handles DELETE /api/v1/backup-targets/{id}.
// AbilityWriteSensitive, same as create: deleting the row that gates
// access to a set of live bucket credentials is a sensitive operation
// even though this handler itself never touches the credential values.
//
// Known gap, left honest rather than silently incomplete:
// internal/secrets.Manager has no delete/revoke operation today (only
// SetValue/Resolve/Exists), so the access_key_id/secret_access_key this
// target's SetValue calls wrote remain in the secrets store,
// unreferenced and unreachable through this API but not actually erased
// at rest. Adding secret deletion is its own change to
// internal/secrets, deliberately not bundled into this one.
func (rt *Router) handleDeleteBackupTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	err := rt.backupTargets.DeleteBackupTarget(r.Context(), id)
	if errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusNotFound, "backup target not found")
		return
	}
	if err != nil {
		// Most likely cause: migrations/0018's foreign key from
		// backup_history still references this target (see
		// store.DeleteBackupTarget's own doc comment). A 409, not a
		// 500: the caller did something the current state of the
		// system won't allow, not something this server got wrong.
		writeError(w, http.StatusConflict, "backup target still has backup history; delete is blocked until that's addressed")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleTestBackupTarget handles POST /api/v1/backup-targets/{id}/test:
// probes the stored credentials against the target's configured bucket
// with a HeadBucket call (internal/backup.S3Tester), no upload or delete
// involved. Catches a bad or stale credential, or a renamed/deleted
// bucket, before the next scheduled backup fails against it silently,
// the same "verify a stored credential by actually using it, once, on
// demand" shape handleTestRegistryCredential already establishes for
// registry credentials.
func (rt *Router) handleTestBackupTarget(w http.ResponseWriter, r *http.Request) {
	if rt.backupTargetTester == nil {
		writeError(w, http.StatusNotImplemented, "backup target testing is not configured on this control plane (no master key set)")
		return
	}

	id := r.PathValue("id")
	if _, err := rt.backupTargets.GetBackupTarget(r.Context(), id); errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusNotFound, "backup target not found")
		return
	} else if err != nil {
		rt.logger.Error("api: test backup target: load failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), backupTargetTestTimeout)
	defer cancel()
	if err := rt.backupTargetTester.TestTarget(ctx, id); err != nil {
		writeError(w, http.StatusBadGateway, backupTargetTestErrorMessage(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// backupTargetTestErrorMessage classifies a failed TestTarget error into
// the two specific reasons HeadBucket's own contract can actually
// distinguish (see api_op_HeadBucket.go's doc comment: a generic 403 for
// a rejected credential, a generic 404 for a missing bucket, no body
// either way) versus everything else (endpoint unreachable, TLS
// failure, timeout). Never includes the access key or secret: the
// wrapped error carries only the endpoint's own HTTP response, which
// never echoes back what it was given, the same guarantee
// registryAuthTestErrorMessage's own doc comment relies on.
func backupTargetTestErrorMessage(err error) string {
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) {
		switch respErr.HTTPStatusCode() {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "authentication rejected by the storage endpoint: check the access key id and secret access key"
		case http.StatusNotFound:
			return "bucket not found at the configured endpoint: check the bucket name, region, and endpoint"
		}
	}
	return fmt.Sprintf("could not reach backup target: %s", err.Error())
}

// randomBackupTargetID mirrors randomNodeJoinTokenID's exact shape
// (internal/api/nodes.go): 9 random bytes, URL-safe base64, a short
// type prefix. Duplicated rather than shared, the same "different
// resource, different ID space, nothing depends on them being
// interchangeable" reasoning randomNodeJoinTokenID's own doc comment
// gives for not sharing with randomTokenID either.
func randomBackupTargetID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate backup target id: %w", err)
	}
	return "bkt_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
