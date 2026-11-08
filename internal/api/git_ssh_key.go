package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/gitsshkey"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/sourcecache"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	gitSSHPrivateKey = "ssh_private_key"
	gitSSHHostKey    = "ssh_host_key"
	envGitSSHScanTTL = "APP_GIT_SSH_SCAN_TIMEOUT"
	defaultSSHScan   = 10 * time.Second
)

// gitSSHKeyResource is the wire shape: the public half and fingerprints only.
// The private key is never serialized anywhere.
type gitSSHKeyResource struct {
	PublicKey       string `json:"public_key"`
	Fingerprint     string `json:"fingerprint"`
	HostFingerprint string `json:"host_fingerprint,omitempty"`
	HostKeyPinned   bool   `json:"host_key_pinned"`
}

type setGitSSHKeyRequest struct {
	// HostKey is the server's public host key in authorized_keys format,
	// obtained out of band. Required unless TrustOnFirstUse is set.
	HostKey string `json:"host_key"`
	// TrustOnFirstUse scans the host once and pins what it presents; the
	// response shows the fingerprint so the user can verify it.
	TrustOnFirstUse bool `json:"trust_on_first_use"`
}

type sshKeyDeleter interface {
	DeleteValue(ctx context.Context, serviceName, envKey string) error
}

// hasSSHDeployKey reports whether name has a deploy key and repoURL is an SSH URL.
func (rt *Router) hasSSHDeployKey(ctx context.Context, name, repoURL string) bool {
	if rt.gitSourceSecrets == nil || !gitsshkey.IsSSHURL(repoURL) {
		return false
	}
	ok, err := rt.gitSourceSecrets.Exists(ctx, store.GitSourceSecretsKey(name), gitSSHPrivateKey)
	return err == nil && ok
}

// withSSHDeployKey attaches the app's deploy key auth to ctx for an SSH repo
// URL. If the key is stored but cannot be used, the context carries an auth
// error so the fetch fails; it never falls back to the host's ssh-agent. With
// no stored key the source cache itself refuses SSH URLs lacking auth.
func (rt *Router) withSSHDeployKey(ctx context.Context, repoURL string) context.Context {
	scope := sourcecache.ScopeOf(ctx)
	name, ok := strings.CutPrefix(scope, "app:")
	if !ok || !rt.hasSSHDeployKey(ctx, name, repoURL) {
		return ctx
	}
	secretsKey := store.GitSourceSecretsKey(name)
	priv, err := rt.gitSourceSecrets.Resolve(ctx, secretsKey, gitSSHPrivateKey)
	if err != nil {
		return sourcecache.WithAuthError(ctx, errSSHKeyUnusable)
	}
	host, err := rt.gitSourceSecrets.Resolve(ctx, secretsKey, gitSSHHostKey)
	if err != nil {
		return sourcecache.WithAuthError(ctx, errSSHKeyUnusable)
	}
	auth, err := gitsshkey.AuthMethod([]byte(priv), host)
	if err != nil {
		return sourcecache.WithAuthError(ctx, errSSHKeyUnusable)
	}
	return sourcecache.WithAuth(ctx, auth)
}

// errSSHKeyUnusable carries no detail so nothing about the key can leak.
var errSSHKeyUnusable = errors.New("ssh deploy key or pinned host key is missing or unreadable; set the key again")

func (rt *Router) sshScanAllow(ctx context.Context) []string {
	if rt.runtimeSettings == nil {
		return nil
	}
	return rt.runtimeSettings.List(ctx, runtimesettings.KeyGitSSHScanAllowInternal)
}

func (rt *Router) sshKeyResource(ctx context.Context, name string) (gitSSHKeyResource, bool, error) {
	secretsKey := store.GitSourceSecretsKey(name)
	has, err := rt.gitSourceSecrets.Exists(ctx, secretsKey, gitSSHPrivateKey)
	if err != nil || !has {
		return gitSSHKeyResource{}, false, err
	}
	priv, err := rt.gitSourceSecrets.Resolve(ctx, secretsKey, gitSSHPrivateKey)
	if err != nil {
		return gitSSHKeyResource{}, false, err
	}
	pub, err := gitsshkey.PublicKeyOf([]byte(priv))
	if err != nil {
		return gitSSHKeyResource{}, false, err
	}
	res := gitSSHKeyResource{PublicKey: pub}
	res.Fingerprint, _ = gitsshkey.Fingerprint(pub)
	if hasHost, _ := rt.gitSourceSecrets.Exists(ctx, secretsKey, gitSSHHostKey); hasHost {
		if host, err := rt.gitSourceSecrets.Resolve(ctx, secretsKey, gitSSHHostKey); err == nil {
			first, _, _ := strings.Cut(strings.TrimSpace(host), "\n")
			if fp, err := gitsshkey.Fingerprint(first); err == nil {
				res.HostFingerprint, res.HostKeyPinned = fp, true
			}
		}
	}
	return res, true, nil
}

// handleGetGitSSHKey handles GET /api/v1/apps/{name}/git-source/ssh-key.
func (rt *Router) handleGetGitSSHKey(w http.ResponseWriter, r *http.Request) {
	if rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "git sources are not configured on this control plane (no master key set)")
		return
	}
	name := r.PathValue("name")
	res, found, err := rt.sshKeyResource(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: read git ssh key failed", err, slog.String("name", name))
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "no SSH deploy key for this app")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleSetGitSSHKey handles POST /api/v1/apps/{name}/git-source/ssh-key: it
// mints a fresh ed25519 key (replacing any previous one) and pins the host key.
func (rt *Router) handleSetGitSSHKey(w http.ResponseWriter, r *http.Request) {
	if rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "git sources are not configured on this control plane (no master key set)")
		return
	}
	name := r.PathValue("name")
	var req setGitSSHKeyRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	gs, err := rt.gitSources.GetGitSource(r.Context(), name)
	if errors.Is(err, store.ErrGitSourceNotFound) || (err == nil && gs == nil) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	}
	if err != nil {
		rt.internalError(w, "api: set git ssh key: load git source failed", err, slog.String("name", name))
		return
	}
	hostPort, err := gitsshkey.Host(gs.RepoURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "the git source repo_url is not an ssh url")
		return
	}

	hostKey := strings.TrimSpace(req.HostKey)
	switch {
	case hostKey != "":
		if _, err := gitsshkey.HostKeyCallback(hostKey); err != nil {
			writeError(w, http.StatusBadRequest, "host_key must be public keys in authorized_keys format, one per line")
			return
		}
	case req.TrustOnFirstUse:
		hostKey, err = gitsshkey.ScanHostKey(r.Context(), hostPort, envDuration(envGitSSHScanTTL, defaultSSHScan), rt.sshScanAllow(r.Context()))
		if errors.Is(err, gitsshkey.ErrBlockedScanTarget) {
			writeError(w, http.StatusBadRequest, "that host resolves to an internal address; paste its host_key instead, or list the network in the git.ssh_scan_allow_internal setting")
			return
		}
		if err != nil {
			writeError(w, http.StatusBadGateway, "could not read the host key from "+hostPort)
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "host_key or trust_on_first_use is required: host keys are always pinned")
		return
	}

	pair, err := gitsshkey.Generate()
	if err != nil {
		rt.internalError(w, "api: set git ssh key: generate failed", err, slog.String("name", name))
		return
	}
	secretsKey := store.GitSourceSecretsKey(name)
	if err := rt.gitSourceSecrets.SetValue(r.Context(), secretsKey, gitSSHPrivateKey, string(pair.PrivatePEM)); err != nil {
		rt.internalError(w, "api: set git ssh key: save private key failed", err, slog.String("name", name))
		return
	}
	if err := rt.gitSourceSecrets.SetValue(r.Context(), secretsKey, gitSSHHostKey, hostKey); err != nil {
		rt.internalError(w, "api: set git ssh key: save host key failed", err, slog.String("name", name))
		return
	}
	res, _, err := rt.sshKeyResource(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: set git ssh key: read back failed", err, slog.String("name", name))
		return
	}
	rt.logger.Info("api: git ssh deploy key set", slog.String("name", name), slog.String("fingerprint", res.Fingerprint))
	writeJSON(w, http.StatusOK, res)
}

// handleDeleteGitSSHKey handles DELETE /api/v1/apps/{name}/git-source/ssh-key.
func (rt *Router) handleDeleteGitSSHKey(w http.ResponseWriter, r *http.Request) {
	del, ok := rt.gitSourceSecrets.(sshKeyDeleter)
	if !ok {
		writeError(w, http.StatusNotImplemented, "git sources are not configured on this control plane (no master key set)")
		return
	}
	name := r.PathValue("name")
	secretsKey := store.GitSourceSecretsKey(name)
	for _, k := range []string{gitSSHPrivateKey, gitSSHHostKey} {
		if err := del.DeleteValue(r.Context(), secretsKey, k); err != nil {
			rt.internalError(w, "api: delete git ssh key failed", err, slog.String("name", name))
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
