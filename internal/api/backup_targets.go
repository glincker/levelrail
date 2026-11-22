package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/fielddesc"
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
	RetireBackupTarget(ctx context.Context, id string) (store.RetireResult, error)
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
	// EncryptDumps is true when new database dumps to this target are age-sealed.
	EncryptDumps bool `json:"encrypt_dumps"`
	// LocalRoot is the read-only APP_BACKUP_LOCAL_ROOT a local target lives under; empty for other providers.
	LocalRoot string `json:"local_root,omitempty"`
	// HostKey is the pinned SSH host key ("type SHA256:...") of an sftp target; public, never a secret.
	HostKey string `json:"host_key,omitempty"`
}

// backupProviderLocal is the API-only provider name for a host directory target.
const backupProviderLocal = "local"

// backupProviderWebDAV is the API-only provider name for a WebDAV share; stored as custom with a dav+ endpoint.
const backupProviderWebDAV = "webdav"

// backupProviderSFTP is the API-only provider name for an SFTP server; stored as custom with an sftp+ endpoint.
const backupProviderSFTP = "sftp"

// sftpKeyAuthMarker fills the password secret of a key-auth target so the shared resolvers keep working.
const sftpKeyAuthMarker = "private-key"

// validateSFTPTarget checks host, remote path and the pinned host key. Pinning is never implicit:
// host_key must come from the user, typically after reviewing the scan result.
func validateSFTPTarget(ctx context.Context, name, endpoint, dir, hostKey string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if _, _, err := backup.SFTPHostPort(endpoint); err != nil {
		return err
	}
	if err := backup.ValidateSFTPHost(ctx, endpoint); err != nil {
		return err
	}
	if _, err := backup.SFTPBasePath(dir); err != nil {
		return err
	}
	if _, _, err := backup.ParseSFTPHostKey(hostKey); err != nil {
		return errors.New("host_key is required: scan the server, check the fingerprint and pass it as \"ssh-ed25519 SHA256:...\"")
	}
	return nil
}

// validateSFTPAuth needs a username plus exactly one of password or private key.
func validateSFTPAuth(user, password, privateKey, passphrase string) error {
	if user == "" {
		return errors.New("access_key_id (the SSH username) is required")
	}
	if (password == "") == (privateKey == "") {
		return errors.New("set exactly one of secret_access_key (password) or private_key")
	}
	if privateKey != "" {
		return backup.ValidateSFTPPrivateKey(privateKey, passphrase)
	}
	return nil
}

// sftpStored maps an API sftp target onto the stored custom provider shape; the pinned key rides in region.
func sftpStored(t *store.BackupTarget, endpoint, dir, hostKey string) {
	base, _ := backup.SFTPBasePath(dir)
	host, port, _ := backup.SFTPHostPort(endpoint)
	algo, fp, _ := backup.ParseSFTPHostKey(hostKey)
	t.Provider, t.Endpoint, t.Region, t.Bucket = store.BackupProviderCustom, store.SFTPEndpointScheme+net.JoinHostPort(host, port), algo+" "+fp, base
}

// setSFTPSecrets stores the credential set, blanking the half that is not in use.
func (rt *Router) setSFTPSecrets(ctx context.Context, id, user, password, privateKey, passphrase string) error {
	if privateKey != "" {
		password = sftpKeyAuthMarker
	}
	key := store.BackupTargetSecretsKey(id)
	for _, kv := range [][2]string{{"private_key", privateKey}, {"key_passphrase", passphrase}, {"access_key_id", user}, {"secret_access_key", password}} {
		if err := rt.backupSecrets.SetValue(ctx, key, kv[0], kv[1]); err != nil {
			return fmt.Errorf("set %s: %w", kv[0], err)
		}
	}
	return nil
}

// validateWebDAVTarget checks the https endpoint (private hosts only when the WebDAV private network setting is on)
// and the directory path. username and password reuse the access key fields.
func validateWebDAVTarget(ctx context.Context, name, endpoint, dir, caBundle string) error {
	if name == "" {
		return errors.New("name is required")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && (u.Scheme != "http" || !backup.WebDAVAllowPrivate())) {
		return errors.New("endpoint must be an https URL without embedded credentials")
	}
	if err := backup.ValidateWebDAVHost(ctx, endpoint); err != nil {
		return err
	}
	if strings.Contains(dir, "..") {
		return errors.New("bucket (the remote directory) must not contain '..'")
	}
	if caBundle != "" {
		if err := backup.ValidateCABundle(caBundle); err != nil {
			return err
		}
	}
	return nil
}

// webDAVStored maps an API webdav target onto the stored custom provider shape.
func webDAVStored(t *store.BackupTarget, endpoint, dir string) {
	t.Provider, t.Endpoint, t.Region = store.BackupProviderCustom, store.WebDAVEndpointScheme+endpoint, ""
	t.Bucket = strings.Trim(path.Clean("/"+dir), "/")
}

func toBackupTargetResource(t store.BackupTarget) backupTargetResource {
	provider, endpoint, localRoot, region, hostKey := t.Provider, t.Endpoint, "", t.Region, ""
	if strings.HasPrefix(t.Endpoint, store.SFTPEndpointScheme) {
		provider, endpoint, region, hostKey = backupProviderSFTP, strings.TrimPrefix(t.Endpoint, store.SFTPEndpointScheme), "", t.Region
	}
	if strings.HasPrefix(t.Endpoint, store.WebDAVEndpointScheme) {
		provider, endpoint = backupProviderWebDAV, strings.TrimPrefix(t.Endpoint, store.WebDAVEndpointScheme)
	}
	if strings.HasPrefix(t.Endpoint, store.LocalEndpointScheme) {
		provider, endpoint = backupProviderLocal, ""
		localRoot = backup.ResolvedLocalRoot()
	}
	return backupTargetResource{
		ID:           t.ID,
		Name:         t.Name,
		Provider:     provider,
		Endpoint:     endpoint,
		Region:       region,
		Bucket:       t.Bucket,
		CreatedAt:    t.CreatedAt,
		EncryptDumps: t.EncryptDumps,
		LocalRoot:    localRoot,
		HostKey:      hostKey,
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
	// EncryptDumps defaults to on when this control plane has a backup key.
	EncryptDumps *bool `json:"encrypt_dumps,omitempty"`
	// CABundle is an optional write-only PEM bundle for webdav targets, stored as a secret.
	CABundle string `json:"ca_bundle,omitempty"`
	// PrivateKey and PrivateKeyPassphrase are write-only sftp key credentials; HostKey is the pinned server key.
	PrivateKey           string `json:"private_key,omitempty"`
	PrivateKeyPassphrase string `json:"private_key_passphrase,omitempty"`
	HostKey              string `json:"host_key,omitempty"`
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
func validateBackupTargetFields(name, provider, endpoint, region, bucket string) error {
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
	// Internal forms are only produced server side by the matching provider paths.
	low := strings.ToLower(strings.TrimSpace(endpoint))
	for _, internal := range []string{store.WebDAVEndpointScheme, store.LocalEndpointScheme, store.AgentEndpointScheme, store.SFTPEndpointScheme} {
		if strings.HasPrefix(low, internal) {
			return errors.New("endpoint must be an http or https URL")
		}
	}
	if endpoint == "" && provider != store.BackupProviderAWS {
		return errors.New("endpoint is required for r2 and custom providers")
	}
	if region == "" && provider == store.BackupProviderCustom {
		return fmt.Errorf("region is required for the %s provider", provider)
	}
	return nil
}

// backupTargetDestination reduces a stored target to the parts that decide where its
// credentials are sent: provider kind, scheme, host and effective port.
func backupTargetDestination(provider, endpoint, region string) string {
	kind, raw := provider, endpoint
	switch {
	case strings.HasPrefix(endpoint, store.SFTPEndpointScheme):
		host, port, _ := backup.SFTPHostPort(endpoint)
		return backupProviderSFTP + "|" + strings.ToLower(host) + "|" + port + "|" + region
	case strings.HasPrefix(endpoint, store.WebDAVEndpointScheme):
		kind, raw = backupProviderWebDAV, strings.TrimPrefix(endpoint, store.WebDAVEndpointScheme)
	case strings.HasPrefix(endpoint, store.LocalEndpointScheme):
		return backupProviderLocal
	case strings.HasPrefix(endpoint, store.AgentEndpointScheme):
		return endpoint
	}
	u, err := url.Parse(raw)
	if err != nil || raw == "" {
		return kind + "|" + raw
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"https": "443", "http": "80"}[strings.ToLower(u.Scheme)]
	}
	return kind + "|" + strings.ToLower(u.Scheme) + "|" + strings.ToLower(u.Hostname()) + "|" + port
}

// descriptorProblem runs the shared field registry over the request values so required
// and select errors read the same on every surface. Skipped keys are optional credentials.
func descriptorProblem(values map[string]string, skip ...string) error {
	res, ok := fielddesc.Get(fielddesc.BackupTarget)
	if !ok {
		return nil
	}
	if values["private_key"] != "" {
		values["auth"] = "key"
	} else {
		values["auth"] = "password"
	}
	if msg := fielddesc.FirstProblem(res.ValidateExcept(values, skip...)); msg != "" {
		return errors.New(msg)
	}
	return nil
}

func validateCreateBackupTargetRequest(ctx context.Context, req createBackupTargetRequest) error {
	if err := descriptorProblem(map[string]string{
		"name": req.Name, "provider": req.Provider, "endpoint": req.Endpoint, "region": req.Region, "bucket": req.Bucket,
		"access_key_id": req.AccessKeyID, "secret_access_key": req.SecretAccessKey, "private_key": req.PrivateKey, "host_key": req.HostKey,
	}); err != nil {
		return err
	}
	if req.Provider == backupProviderSFTP {
		if err := validateSFTPTarget(ctx, req.Name, req.Endpoint, req.Bucket, req.HostKey); err != nil {
			return err
		}
		return validateSFTPAuth(req.AccessKeyID, req.SecretAccessKey, req.PrivateKey, req.PrivateKeyPassphrase)
	}
	if req.Provider == backupProviderWebDAV {
		if err := validateWebDAVTarget(ctx, req.Name, req.Endpoint, req.Bucket, req.CABundle); err != nil {
			return err
		}
		if req.AccessKeyID == "" || req.SecretAccessKey == "" {
			return errors.New("access_key_id (username) and secret_access_key (password) are required")
		}
		return nil
	}
	if req.Provider == backupProviderLocal {
		if req.Name == "" {
			return errors.New("name is required")
		}
		if backup.ResolvedLocalRoot() == "" {
			return errors.New("local targets need " + backup.EnvLocalRoot + " (or APP_DATA_DIR) set to an absolute directory on the control plane")
		}
		_, err := backup.CleanLocalPath(req.Bucket)
		return err
	}
	if err := validateBackupTargetFields(req.Name, req.Provider, req.Endpoint, req.Region, req.Bucket); err != nil {
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
	CABundle        string `json:"ca_bundle,omitempty"`
	// ClearCABundle drops the stored bundle; ca_bundle is write-only so it cannot be sent empty.
	ClearCABundle bool `json:"clear_ca_bundle,omitempty"`
	// Write-only sftp credentials and the pinned host key, see createBackupTargetRequest.
	PrivateKey           string `json:"private_key,omitempty"`
	PrivateKeyPassphrase string `json:"private_key_passphrase,omitempty"`
	HostKey              string `json:"host_key,omitempty"`
}

func validateUpdateBackupTargetRequest(ctx context.Context, req updateBackupTargetRequest) error {
	if err := descriptorProblem(map[string]string{
		"name": req.Name, "provider": req.Provider, "endpoint": req.Endpoint, "region": req.Region, "bucket": req.Bucket,
		"private_key": req.PrivateKey, "host_key": req.HostKey,
	}, "access_key_id", "secret_access_key", "private_key"); err != nil {
		return err
	}
	if req.Provider == backupProviderSFTP {
		if err := validateSFTPTarget(ctx, req.Name, req.Endpoint, req.Bucket, req.HostKey); err != nil {
			return err
		}
		if req.AccessKeyID == "" && (req.SecretAccessKey != "" || req.PrivateKey != "") {
			return errors.New("access_key_id (the SSH username) must be sent with new credentials")
		}
		if req.AccessKeyID != "" {
			return validateSFTPAuth(req.AccessKeyID, req.SecretAccessKey, req.PrivateKey, req.PrivateKeyPassphrase)
		}
		return nil
	}
	if req.Provider == backupProviderWebDAV {
		if err := validateWebDAVTarget(ctx, req.Name, req.Endpoint, req.Bucket, req.CABundle); err != nil {
			return err
		}
		if (req.AccessKeyID == "") != (req.SecretAccessKey == "") {
			return errors.New("access_key_id and secret_access_key must be set together to rotate credentials")
		}
		return nil
	}
	if err := validateBackupTargetFields(req.Name, req.Provider, req.Endpoint, req.Region, req.Bucket); err != nil {
		return err
	}
	if (req.AccessKeyID == "") != (req.SecretAccessKey == "") {
		return errors.New("access_key_id and secret_access_key must be set together to rotate credentials")
	}
	return nil
}

// backupTargetsHidden is true when strict isolation hides the ownerless backup
// targets from the caller; only instance wide callers (root) see them.
func (rt *Router) backupTargetsHidden(r *http.Request) (bool, error) {
	v, err := rt.strictViewFor(r)
	if err != nil {
		return false, err
	}
	return v != nil && !v.instanceWide, nil
}

// handleListBackupTargets handles GET /api/v1/backup-targets.
func (rt *Router) handleListBackupTargets(w http.ResponseWriter, r *http.Request) {
	hidden, err := rt.backupTargetsHidden(r)
	if err != nil {
		rt.internalError(w, "api: list backup targets: resolve caller", err)
		return
	}
	if hidden {
		writeJSON(w, http.StatusOK, []backupTargetResource{})
		return
	}
	targets, err := rt.backupTargets.ListBackupTargets(r.Context())
	if err != nil {
		rt.internalError(w, "api: list backup targets failed", err)
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
	if hidden, err := rt.backupTargetsHidden(r); err != nil {
		rt.internalError(w, "api: get backup target: resolve caller", err)
		return
	} else if hidden {
		writeError(w, http.StatusNotFound, "backup target not found")
		return
	}

	t, err := rt.backupTargets.GetBackupTarget(r.Context(), id)
	if errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusNotFound, "backup target not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get backup target failed", err, slog.String("id", id))
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
	if !decodeReq(w, r, &req) {
		return
	}
	if err := validateCreateBackupTargetRequest(r.Context(), req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	encryptDumps, ok := rt.resolveCreateEncryption(w, req.EncryptDumps)
	if !ok {
		return
	}

	id, err := randomBackupTargetID()
	if err != nil {
		rt.internalError(w, "api: create backup target: generate id failed", err)
		return
	}

	target := store.BackupTarget{
		ID:           id,
		Name:         req.Name,
		Provider:     req.Provider,
		Endpoint:     req.Endpoint,
		Region:       req.Region,
		Bucket:       req.Bucket,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		EncryptDumps: encryptDumps,
	}
	switch req.Provider {
	case backupProviderLocal:
		clean, _ := backup.CleanLocalPath(req.Bucket)
		target.Provider, target.Endpoint, target.Region, target.Bucket = store.BackupProviderCustom, store.LocalEndpointScheme, "", clean
	case backupProviderSFTP:
		sftpStored(&target, req.Endpoint, req.Bucket, req.HostKey)
		if err := rt.setSFTPSecrets(r.Context(), id, req.AccessKeyID, req.SecretAccessKey, req.PrivateKey, req.PrivateKeyPassphrase); err != nil {
			rt.internalError(w, "api: create backup target: set sftp credentials failed", err, slog.String("id", id))
			return
		}
	default:
		if req.Provider == backupProviderWebDAV {
			webDAVStored(&target, req.Endpoint, req.Bucket)
		}
		secretsKey := store.BackupTargetSecretsKey(id)
		if req.CABundle != "" {
			if err := rt.backupSecrets.SetValue(r.Context(), secretsKey, "ca_bundle", req.CABundle); err != nil {
				rt.internalError(w, "api: create backup target: set ca_bundle failed", err, slog.String("id", id))
				return
			}
		}
		if err := rt.backupSecrets.SetValue(r.Context(), secretsKey, "access_key_id", req.AccessKeyID); err != nil {
			rt.internalError(w, "api: create backup target: set access_key_id failed", err, slog.String("id", id))
			return
		}
		if err := rt.backupSecrets.SetValue(r.Context(), secretsKey, "secret_access_key", req.SecretAccessKey); err != nil {
			rt.internalError(w, "api: create backup target: set secret_access_key failed", err, slog.String("id", id))
			return
		}
	}
	if err := rt.backupTargets.SaveBackupTarget(r.Context(), target); err != nil {
		rt.internalError(w, "api: create backup target: save failed", err, slog.String("id", id))
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

	current, err := rt.backupTargets.GetBackupTarget(r.Context(), id)
	if errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusNotFound, "backup target not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: update backup target: load failed", err, slog.String("id", id))
		return
	}

	var req updateBackupTargetRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if err := validateUpdateBackupTargetRequest(r.Context(), req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	provider, endpoint, region, bucket := req.Provider, req.Endpoint, req.Region, req.Bucket
	if req.Provider == backupProviderWebDAV {
		var t store.BackupTarget
		webDAVStored(&t, req.Endpoint, req.Bucket)
		provider, endpoint, region, bucket = t.Provider, t.Endpoint, t.Region, t.Bucket
	}
	if req.Provider == backupProviderSFTP {
		var t store.BackupTarget
		sftpStored(&t, req.Endpoint, req.Bucket, req.HostKey)
		provider, endpoint, region, bucket = t.Provider, t.Endpoint, t.Region, t.Bucket
	}
	// Stored credentials only follow an unchanged destination; a new one needs its own.
	if req.AccessKeyID == "" && backupTargetDestination(current.Provider, current.Endpoint, current.Region) != backupTargetDestination(provider, endpoint, region) {
		writeError(w, http.StatusBadRequest, "changing the provider or endpoint requires access_key_id and secret_access_key in the same request")
		return
	}
	if req.Provider == backupProviderWebDAV {
		if (req.CABundle != "" || req.ClearCABundle) && req.AccessKeyID == "" {
			writeError(w, http.StatusBadRequest, "changing the CA bundle requires access_key_id and secret_access_key in the same request")
			return
		}
		if (req.CABundle != "" || req.ClearCABundle) && rt.backupSecrets != nil {
			bundle := req.CABundle
			if req.ClearCABundle {
				bundle = ""
			}
			if err := rt.backupSecrets.SetValue(r.Context(), store.BackupTargetSecretsKey(id), "ca_bundle", bundle); err != nil {
				rt.internalError(w, "api: update backup target: set ca_bundle failed", err, slog.String("id", id))
				return
			}
		}
	}
	if req.AccessKeyID != "" && req.Provider == backupProviderSFTP {
		if rt.backupSecrets == nil {
			writeError(w, http.StatusNotImplemented, "backup targets are not configured on this control plane (no master key set)")
			return
		}
		if err := rt.setSFTPSecrets(r.Context(), id, req.AccessKeyID, req.SecretAccessKey, req.PrivateKey, req.PrivateKeyPassphrase); err != nil {
			rt.internalError(w, "api: update backup target: set sftp credentials failed", err, slog.String("id", id))
			return
		}
	} else if req.AccessKeyID != "" {
		if rt.backupSecrets == nil {
			writeError(w, http.StatusNotImplemented, "backup targets are not configured on this control plane (no master key set)")
			return
		}
		secretsKey := store.BackupTargetSecretsKey(id)
		if err := rt.backupSecrets.SetValue(r.Context(), secretsKey, "access_key_id", req.AccessKeyID); err != nil {
			rt.internalError(w, "api: update backup target: set access_key_id failed", err, slog.String("id", id))
			return
		}
		if err := rt.backupSecrets.SetValue(r.Context(), secretsKey, "secret_access_key", req.SecretAccessKey); err != nil {
			rt.internalError(w, "api: update backup target: set secret_access_key failed", err, slog.String("id", id))
			return
		}
	}

	if err := rt.backupTargets.UpdateBackupTarget(r.Context(), id, req.Name, provider, endpoint, region, bucket); err != nil {
		if errors.Is(err, store.ErrBackupTargetNotFound) {
			writeError(w, http.StatusNotFound, "backup target not found")
			return
		}
		rt.internalError(w, "api: update backup target failed", err, slog.String("id", id))
		return
	}

	updated, err := rt.backupTargets.GetBackupTarget(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: update backup target: reload after update failed", err, slog.String("id", id))
		return
	}
	writeJSON(w, http.StatusOK, toBackupTargetResource(updated))
}

// handleDeleteBackupTarget handles DELETE /api/v1/backup-targets/{id}.
// AbilityWriteSensitive, same as create: deleting the row that gates
// access to a set of live bucket credentials is a sensitive operation
// even though this handler itself never reads the credential values.
// The retire wipes the stored credentials in the same transaction.
func (rt *Router) handleDeleteBackupTarget(w http.ResponseWriter, r *http.Request) {
	rt.retireBackupTarget(w, r, r.PathValue("id"), "backup target")
}

// retireBackupTarget answers 409 with the live references, or detaches the
// history of deleted resources and records how many rows were detached.
func (rt *Router) retireBackupTarget(w http.ResponseWriter, r *http.Request, id, noun string) {
	res, err := rt.backupTargets.RetireBackupTarget(r.Context(), id)
	var inUse *store.BackupTargetInUseError
	switch {
	case errors.Is(err, store.ErrBackupTargetNotFound):
		writeError(w, http.StatusNotFound, noun+" not found")
		return
	case errors.As(err, &inUse):
		writeError(w, http.StatusConflict, noun+" is still in use by "+strings.Join(inUse.Blockers, "; ")+"; remove or delete those first (for backup history, delete the listed backups or the database itself)")
		return
	case err != nil && strings.Contains(err.Error(), "FOREIGN KEY"):
		writeError(w, http.StatusConflict, noun+" is still referenced by other records; remove those first")
		return
	case err != nil:
		rt.internalError(w, "api: delete "+noun+": retire failed", err, slog.String("id", id))
		return
	}
	rt.auditBackupTargetDeleted(r, id, res)
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) auditBackupTargetDeleted(r *http.Request, id string, res store.RetireResult) {
	if rt.auditLog == nil {
		return
	}
	entryID, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: backup target delete audit id failed", slog.String("error", err.Error()))
		return
	}
	a := rt.accessActor(r)
	detail, _ := json.Marshal(map[string]any{"target_id": id, "detached_history_rows": res.Detached, "pruned_failed_attempts": res.Pruned})
	entry := store.AuditEntry{
		ID: entryID, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWriteSensitive, Method: r.Method, Path: r.URL.Path, StatusCode: http.StatusNoContent,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     "backup_target.deleted", Detail: string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: backup target delete audit save failed", slog.String("error", err.Error()))
	}
}

type sftpHostKeyRequest struct {
	Endpoint string `json:"endpoint"`
}

type sftpHostKeyResponse struct {
	HostKey     string `json:"host_key"`
	KeyType     string `json:"key_type"`
	Fingerprint string `json:"fingerprint"`
}

// handleScanSFTPHostKey handles POST /api/v1/backup-targets/sftp-host-key: it reads the key a server
// offers so the user can verify it out of band and pin it. It sends no credentials and stores nothing.
func (rt *Router) handleScanSFTPHostKey(w http.ResponseWriter, r *http.Request) {
	if hidden, err := rt.backupTargetsHidden(r); err != nil {
		rt.internalError(w, "api: scan sftp host key: resolve caller", err)
		return
	} else if hidden {
		writeError(w, http.StatusNotFound, "backup target not found")
		return
	}
	var req sftpHostKeyRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if _, _, err := backup.SFTPHostPort(req.Endpoint); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !rt.allowSFTPScan(r) {
		writeError(w, http.StatusTooManyRequests, "too many host key scans, try again in a minute")
		return
	}
	// One answer for every failure, so the endpoint cannot be used to map internal networks.
	fail := func(err error) {
		rt.logger.Debug("api: sftp host key scan failed", slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, sftpScanFailed)
	}
	if err := backup.ValidateSFTPHost(r.Context(), req.Endpoint); err != nil {
		fail(err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), backupTargetTestTimeout)
	defer cancel()
	key, err := backup.ScanSFTPHostKey(ctx, req.Endpoint)
	if err != nil {
		fail(err)
		return
	}
	algo, fp, _ := backup.ParseSFTPHostKey(key)
	writeJSON(w, http.StatusOK, sftpHostKeyResponse{HostKey: key, KeyType: algo, Fingerprint: fp})
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
		rt.internalError(w, "api: test backup target: load failed", err, slog.String("id", id))
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

// BackupTargetProber is the optional richer test: latency and free space.
type BackupTargetProber interface {
	ProbeTarget(ctx context.Context, targetID string) (backup.TestReport, error)
}

// handleProbeBackupTarget handles POST /api/v1/backup-targets/{id}/probe: the test-connection
// check, answering 200 with {ok, latency_ms, free_bytes?} instead of an empty 204.
func (rt *Router) handleProbeBackupTarget(w http.ResponseWriter, r *http.Request) {
	prober, ok := rt.backupTargetTester.(BackupTargetProber)
	if !ok || rt.backupTargetTester == nil {
		writeError(w, http.StatusNotImplemented, "backup target testing is not configured on this control plane (no master key set)")
		return
	}
	id := r.PathValue("id")
	if _, err := rt.backupTargets.GetBackupTarget(r.Context(), id); errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusNotFound, "backup target not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: probe backup target: load failed", err, slog.String("id", id))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), backupTargetTestTimeout)
	defer cancel()
	rep, err := prober.ProbeTarget(ctx, id)
	if err != nil {
		writeError(w, http.StatusBadGateway, backupTargetTestErrorMessage(err))
		return
	}
	writeJSON(w, http.StatusOK, rep)
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
			return "authentication rejected by the storage endpoint: check the access key id and secret access key; the key needs s3:ListBucket on the bucket and s3:PutObject, s3:GetObject and s3:DeleteObject on its objects"
		case http.StatusMovedPermanently, http.StatusBadRequest:
			return "the storage endpoint refused the request, usually because the bucket is in another region: check the region"
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

// handleGetBackupTargetFields handles GET /api/v1/backup-targets/field-descriptors: the one field
// registry the CLI, the dashboard form and the docs are built from.
func (rt *Router) handleGetBackupTargetFields(w http.ResponseWriter, _ *http.Request) {
	res, ok := fielddesc.Get(fielddesc.BackupTarget)
	if !ok {
		rt.internalError(w, "field descriptors", errors.New("field descriptors unavailable"))
		return
	}
	writeJSON(w, http.StatusOK, res)
}
