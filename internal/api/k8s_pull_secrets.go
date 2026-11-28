package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/k8sconn"
	"github.com/glincker/levelrail/internal/secretmask"
	secretsvc "github.com/glincker/levelrail/internal/secrets"
	"github.com/glincker/levelrail/internal/store"
)

const (
	auditK8sPullSecretAdded   = "k8s_pull_secret.added"   //nolint:gosec // audit action name
	auditK8sPullSecretRemoved = "k8s_pull_secret.removed" //nolint:gosec // audit action name
)

var k8sPullSecretNameRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)

// k8sPullSecretStore adds listing and single-value delete to the cluster secrets surface.
type k8sPullSecretStore interface {
	k8sClusterSecrets
	ListKeys(ctx context.Context, serviceName string) ([]store.SecretKeyInfo, error)
	DeleteValue(ctx context.Context, serviceName, envKey string) error
}

// k8sPullNamespace is the sealed namespace holding one value per pull secret. Cluster names
// have no slash, so it never collides with a cluster's own credential namespace.
func k8sPullNamespace(cluster string) string { return store.K8sClusterSecretsKey(cluster) + "/pull" }

// storedPullSecret is the sealed value. It is never returned or logged.
type storedPullSecret struct {
	Registry string `json:"registry"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type k8sPullSecretRequest struct {
	Name     string `json:"name"`
	Registry string `json:"registry"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type k8sPullSecretResource struct {
	Name     string `json:"name"`
	Registry string `json:"registry"`
}

type k8sPullSecretListResponse struct {
	PullSecrets []k8sPullSecretResource `json:"pull_secrets"`
}

func (rt *Router) k8sPullStore() (k8sPullSecretStore, bool) {
	s, ok := rt.nodeProviderSecrets.(k8sPullSecretStore)
	return s, ok && s != nil
}

// k8sPullPrecheck runs the shared gate, finds the cluster and the sealed store.
func (rt *Router) k8sPullPrecheck(w http.ResponseWriter, r *http.Request) (store.K8sCluster, k8sPullSecretStore, bool) {
	if !rt.k8sGate(w, r) {
		return store.K8sCluster{}, nil, false
	}
	c, err := rt.k8sClusterRows.GetK8sCluster(r.Context(), r.PathValue("name"))
	if errors.Is(err, store.ErrK8sClusterNotFound) {
		writeError(w, http.StatusNotFound, "cluster not found")
		return c, nil, false
	}
	if err != nil {
		rt.internalError(w, "api: get k8s cluster failed", err)
		return c, nil, false
	}
	sec, ok := rt.k8sPullStore()
	if !ok {
		writeError(w, http.StatusNotImplemented, k8sSecretsUnavailableMessage(false))
		return c, nil, false
	}
	return c, sec, true
}

func (rt *Router) handleAddK8sPullSecret(w http.ResponseWriter, r *http.Request) {
	c, sec, ok := rt.k8sPullPrecheck(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	lim := k8sconn.LoadLimits(ctx, rt.runtimeSettings)
	var req k8sPullSecretRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, lim.MaxBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	host := k8sconn.NormalizeRegistryHost(req.Registry)
	switch {
	case !k8sPullSecretNameRe.MatchString(req.Name):
		writeError(w, http.StatusBadRequest, "name must be lowercase letters, digits and dashes, up to 63 characters")
		return
	case host == "":
		writeError(w, http.StatusBadRequest, "registry must be a host such as ghcr.io or registry.example.com:5000")
		return
	case req.Username == "" || req.Password == "":
		writeError(w, http.StatusBadRequest, "username and password (or token) are required")
		return
	}
	secretmask.Add(req.Password)
	raw, err := json.Marshal(storedPullSecret{ //nolint:gosec // sealed before storage, never returned
		Registry: host, Username: req.Username, Password: req.Password})
	if err != nil {
		rt.internalError(w, "api: encode k8s pull secret failed", err)
		return
	}
	if err := sec.SetValue(ctx, k8sPullNamespace(c.Name), req.Name, string(raw)); err != nil {
		rt.internalError(w, "api: store k8s pull secret failed", err)
		return
	}
	rt.auditK8sPullSecret(r, auditK8sPullSecretAdded, c.Name, req.Name, host, http.StatusCreated)
	writeJSON(w, http.StatusCreated, k8sPullSecretResource{Name: req.Name, Registry: host})
}

func (rt *Router) handleListK8sPullSecrets(w http.ResponseWriter, r *http.Request) {
	c, sec, ok := rt.k8sPullPrecheck(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	keys, err := sec.ListKeys(ctx, k8sPullNamespace(c.Name))
	if err != nil {
		rt.internalError(w, "api: list k8s pull secrets failed", err)
		return
	}
	out := k8sPullSecretListResponse{PullSecrets: make([]k8sPullSecretResource, 0, len(keys))}
	for _, k := range keys {
		stored, err := rt.loadK8sPullSecret(ctx, sec, c.Name, k.Key)
		if err != nil {
			continue
		}
		out.PullSecrets = append(out.PullSecrets, k8sPullSecretResource{Name: k.Key, Registry: stored.Registry})
	}
	sort.Slice(out.PullSecrets, func(i, j int) bool { return out.PullSecrets[i].Name < out.PullSecrets[j].Name })
	writeJSON(w, http.StatusOK, out)
}

// handleDeleteK8sPullSecret skips the feature gate like disconnect, so stored credentials
// can still be removed after the feature is turned off.
func (rt *Router) handleDeleteK8sPullSecret(w http.ResponseWriter, r *http.Request) {
	if rt.k8sClusterRows == nil {
		writeError(w, http.StatusNotImplemented, "kubernetes clusters are not available on this store")
		return
	}
	c, err := rt.k8sClusterRows.GetK8sCluster(r.Context(), r.PathValue("name"))
	if errors.Is(err, store.ErrK8sClusterNotFound) {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get k8s cluster failed", err)
		return
	}
	sec, ok := rt.k8sPullStore()
	if !ok {
		writeError(w, http.StatusNotImplemented, k8sSecretsUnavailableMessage(false))
		return
	}
	name := r.PathValue("secret")
	stored, err := rt.loadK8sPullSecret(r.Context(), sec, c.Name, name)
	if errors.Is(err, secretsvc.ErrValueNotFound) {
		writeError(w, http.StatusNotFound, "pull secret not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: load k8s pull secret failed", err)
		return
	}
	if err := sec.DeleteValue(r.Context(), k8sPullNamespace(c.Name), name); err != nil {
		rt.internalError(w, "api: delete k8s pull secret failed", err)
		return
	}
	rt.auditK8sPullSecret(r, auditK8sPullSecretRemoved, c.Name, name, stored.Registry, http.StatusNoContent)
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) loadK8sPullSecret(ctx context.Context, sec k8sPullSecretStore, cluster, name string) (storedPullSecret, error) {
	var out storedPullSecret
	raw, err := sec.Resolve(ctx, k8sPullNamespace(cluster), name)
	if err != nil {
		return out, err
	}
	if raw == "" {
		return out, secretsvc.ErrValueNotFound
	}
	secretmask.Add(raw)
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return storedPullSecret{}, errors.New("stored pull secret is unreadable")
	}
	secretmask.Add(out.Password)
	return out, nil
}

// resolveK8sPullSecret finds the credential a deploy asks for and refuses an image from
// another registry, so a token is never sent to a registry it was not issued for.
func (rt *Router) resolveK8sPullSecret(ctx context.Context, cluster, name, image string) (storedPullSecret, int, string) {
	if !k8sPullSecretNameRe.MatchString(name) {
		return storedPullSecret{}, http.StatusBadRequest, "pull_secret must be a lowercase name such as ghcr-main"
	}
	sec, ok := rt.k8sPullStore()
	if !ok {
		return storedPullSecret{}, http.StatusNotImplemented, k8sSecretsUnavailableMessage(false)
	}
	stored, err := rt.loadK8sPullSecret(ctx, sec, cluster, name)
	if errors.Is(err, secretsvc.ErrValueNotFound) {
		return stored, http.StatusBadRequest, "pull_secret " + name + " does not exist on this cluster; add it with clusters pull-secret add"
	}
	if err != nil {
		rt.logger.Warn("api: load k8s pull secret failed", slog.String("cluster", cluster), slog.String("secret", name))
		return stored, http.StatusInternalServerError, "the pull secret could not be loaded"
	}
	if got := k8sconn.ImageRegistryHost(image); got != stored.Registry {
		return stored, http.StatusBadRequest, "the image is on registry " + got + " but pull secret " + name + " is for " + stored.Registry + "; nothing was sent"
	}
	return stored, 0, ""
}

// k8sPulledSecretNames lists the Secret objects a deployment row says we created.
func k8sPulledSecretNames(objects []string) []string {
	var out []string
	for _, o := range objects {
		if n, ok := strings.CutPrefix(o, "Secret/"); ok && n != "" {
			out = append(out, n)
		}
	}
	return out
}

func (rt *Router) auditK8sPullSecret(r *http.Request, action, cluster, name, registry string, status int) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: k8s pull secret audit id failed", slog.String("error", err.Error()))
		return
	}
	ptype, pid, _, perr := rt.callerPrincipal(r)
	if perr != nil {
		ptype, pid = auditActorSystem, "unknown"
	}
	detail, _ := json.Marshal(map[string]any{"cluster": cluster, "name": name, "registry": registry})
	entry := store.AuditEntry{
		ID: id, ActorType: ptype, ActorID: pid, ActorName: pid, Ability: AbilityRoot, Method: r.Method,
		Path: k8sClustersPath + "/" + cluster + "/pull-secrets", StatusCode: status, RemoteAddr: clientIP(r),
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.UserAgent()),
		Action: action, Detail: string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: save k8s pull secret audit failed", slog.String("action", action))
	}
}
