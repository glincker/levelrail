package api

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/GLINCKER/levelrail/internal/k8sconn"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// Audit actions for cluster changes. Rows carry names and hosts, never credentials.
const (
	auditK8sClusterConnected    = "k8s_cluster.connected"
	auditK8sClusterDisconnected = "k8s_cluster.disconnected"
	auditK8sClusterTested       = "k8s_cluster.tested"
)

// Secret keys inside the per-cluster namespace.
const (
	k8sSecretToken      = "token"
	k8sSecretClientKey  = "client_key"
	k8sSecretClientCert = "client_cert"
	k8sSecretCA         = "ca"
)

var k8sClusterNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// K8sClusterRows is the store surface the cluster routes need.
type K8sClusterRows interface {
	ListK8sClusters(ctx context.Context) ([]store.K8sCluster, error)
	GetK8sCluster(ctx context.Context, name string) (store.K8sCluster, error)
	CreateK8sCluster(ctx context.Context, c store.K8sCluster) error
	SetK8sClusterCheck(ctx context.Context, name, class string, at time.Time) error
	DeleteK8sCluster(ctx context.Context, name string) error
}

func k8sClusterStoreOf(s Store) K8sClusterRows {
	r, _ := s.(K8sClusterRows)
	return r
}

// k8sClusterSecrets is what the cluster routes need beyond NodeProviderSecrets.
type k8sClusterSecrets interface {
	NodeProviderSecrets
	DeleteAll(ctx context.Context, serviceName string) error
}

// k8sClusterResource never carries credential material.
type k8sClusterResource struct {
	Name           string     `json:"name"`
	Server         string     `json:"server"`
	AuthMode       string     `json:"auth_mode"`
	CAFingerprint  string     `json:"ca_fingerprint,omitempty"`
	Context        string     `json:"context,omitempty"`
	CreatedBy      string     `json:"created_by,omitempty"`
	CreatedAt      *time.Time `json:"created_at,omitempty"`
	LastCheckedAt  *time.Time `json:"last_checked_at,omitempty"`
	LastCheckClass string     `json:"last_check_class,omitempty"`
}

type k8sClusterListResponse struct {
	Clusters []k8sClusterResource `json:"clusters"`
}

// k8sConnectRequest takes a kubeconfig, or a server with a service-account token and CA.
type k8sConnectRequest struct {
	Name       string `json:"name"`
	Kubeconfig string `json:"kubeconfig,omitempty"`
	Context    string `json:"context,omitempty"`
	Server     string `json:"server,omitempty"`
	Token      string `json:"token,omitempty"`
	CA         string `json:"ca,omitempty"`
}

type k8sTestResponse struct {
	OK           bool   `json:"ok"`
	Class        string `json:"class"`
	ClusterAdmin bool   `json:"cluster_admin,omitempty"`
}

func toK8sClusterResource(c store.K8sCluster) k8sClusterResource {
	res := k8sClusterResource{
		Name: c.Name, Server: c.Server, AuthMode: c.AuthMode, CAFingerprint: c.CAFingerprint,
		Context: c.ContextName, CreatedBy: c.CreatedBy, LastCheckClass: c.LastCheckClass,
	}
	if !c.CreatedAt.IsZero() {
		t := c.CreatedAt
		res.CreatedAt = &t
	}
	if !c.LastCheckedAt.IsZero() {
		t := c.LastCheckedAt
		res.LastCheckedAt = &t
	}
	return res
}

// k8sGate writes the refusal and returns false while the feature is off or the store is missing.
func (rt *Router) k8sGate(w http.ResponseWriter, r *http.Request) bool {
	if rt.runtimeSettings == nil || !rt.runtimeSettings.Bool(r.Context(), runtimesettings.KeyK8sClustersEnabled) {
		writeError(w, http.StatusForbidden, "kubernetes clusters are off; enable the runtime setting k8s.clusters_enabled")
		return false
	}
	if rt.k8sClusterRows == nil {
		writeError(w, http.StatusNotImplemented, "kubernetes clusters are not available on this store")
		return false
	}
	return true
}

func (rt *Router) k8sSecrets() (k8sClusterSecrets, bool) {
	s, ok := rt.nodeProviderSecrets.(k8sClusterSecrets)
	return s, ok && s != nil
}

// k8sGuard allows private and loopback API servers only through the runtime setting.
func (rt *Router) k8sGuard(ctx context.Context) trafficpolicy.EgressGuard {
	allow := k8sconn.AllowPrivateAPI(ctx, rt.runtimeSettings)
	return trafficpolicy.EgressGuard{AllowPrivate: allow, AllowLoopback: allow}
}

func (rt *Router) newK8sClient(ctx context.Context, creds k8sconn.Credentials) (*k8sconn.Client, error) {
	lim := k8sconn.LoadLimits(ctx, rt.runtimeSettings)
	return k8sconn.NewClient(ctx, k8sconn.Config{Creds: creds, Guard: rt.k8sGuard(ctx), Timeout: lim.Timeout, MaxBody: lim.MaxBody})
}

func (rt *Router) handleListK8sClusters(w http.ResponseWriter, r *http.Request) {
	if !rt.k8sGate(w, r) {
		return
	}
	list, err := rt.k8sClusterRows.ListK8sClusters(r.Context())
	if err != nil {
		rt.internalError(w, "api: list k8s clusters failed", err)
		return
	}
	out := k8sClusterListResponse{Clusters: make([]k8sClusterResource, 0, len(list))}
	for _, c := range list {
		out.Clusters = append(out.Clusters, toK8sClusterResource(c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleGetK8sCluster(w http.ResponseWriter, r *http.Request) {
	if !rt.k8sGate(w, r) {
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
	writeJSON(w, http.StatusOK, toK8sClusterResource(c))
}

func (rt *Router) handleConnectK8sCluster(w http.ResponseWriter, r *http.Request) {
	if !rt.k8sGate(w, r) {
		return
	}
	secrets, ok := rt.k8sSecrets()
	if !ok {
		writeError(w, http.StatusNotImplemented, "storing cluster credentials needs the secrets master key")
		return
	}
	ctx := r.Context()
	lim := k8sconn.LoadLimits(ctx, rt.runtimeSettings)
	// JSON escaping can multiply a document, so the body cap is looser than the document cap.
	var req k8sConnectRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, int64(lim.KubeconfigMax)*4+4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !k8sClusterNameRe.MatchString(req.Name) {
		writeError(w, http.StatusBadRequest, "name must be lowercase letters, digits and dashes, up to 63 characters")
		return
	}
	creds, err := k8sCredsFromRequest(req, lim.KubeconfigMax)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Building the client runs the SSRF check once, so a refused address never gets stored.
	if _, err := rt.newK8sClient(ctx, creds); err != nil {
		writeError(w, http.StatusBadRequest, "api server address refused or unusable: "+err.Error())
		return
	}
	if _, err := rt.k8sClusterRows.GetK8sCluster(ctx, req.Name); err == nil {
		writeError(w, http.StatusConflict, "a cluster with this name already exists")
		return
	}
	_, actor, _, _ := rt.callerPrincipal(r)
	row := store.K8sCluster{
		Name: req.Name, Server: creds.Server, ServerName: creds.ServerName, AuthMode: k8sAuthMode(creds),
		CAFingerprint: caFingerprint(creds.CA), ContextName: req.Context, CreatedBy: actor,
	}
	ns := store.K8sClusterSecretsKey(req.Name)
	for key, val := range map[string]string{
		k8sSecretToken: creds.Token, k8sSecretClientKey: string(creds.ClientKey),
		k8sSecretClientCert: string(creds.ClientCert), k8sSecretCA: string(creds.CA),
	} {
		if val == "" {
			continue
		}
		if err := secrets.SetValue(ctx, ns, key, val); err != nil {
			_ = secrets.DeleteAll(context.WithoutCancel(ctx), ns)
			rt.internalError(w, "api: store k8s credential failed", err)
			return
		}
	}
	if err := rt.k8sClusterRows.CreateK8sCluster(ctx, row); err != nil {
		_ = secrets.DeleteAll(context.WithoutCancel(ctx), ns)
		if errors.Is(err, store.ErrK8sClusterExists) {
			writeError(w, http.StatusConflict, "a cluster with this name already exists")
			return
		}
		rt.internalError(w, "api: create k8s cluster failed", err)
		return
	}
	rt.auditK8sCluster(r, auditK8sClusterConnected, row)
	created, err := rt.k8sClusterRows.GetK8sCluster(ctx, req.Name)
	if err != nil {
		created = row
	}
	writeJSON(w, http.StatusCreated, toK8sClusterResource(created))
}

// k8sCredsFromRequest parses the kubeconfig, or takes the direct server and token form.
func k8sCredsFromRequest(req k8sConnectRequest, maxBytes int) (k8sconn.Credentials, error) {
	if req.Kubeconfig != "" {
		return k8sconn.ParseKubeconfig([]byte(req.Kubeconfig), req.Context, maxBytes)
	}
	if req.Server == "" || req.Token == "" {
		return k8sconn.Credentials{}, errors.New("send a kubeconfig, or a server and a service account token")
	}
	creds := k8sconn.Credentials{Server: req.Server, Token: req.Token}
	if req.CA != "" {
		creds.CA = []byte(req.CA)
		if !x509.NewCertPool().AppendCertsFromPEM(creds.CA) {
			return k8sconn.Credentials{}, errors.New("ca holds no pem certificate")
		}
	}
	return creds, nil
}

func k8sAuthMode(c k8sconn.Credentials) string {
	if c.Token != "" {
		return "token"
	}
	return "client_cert"
}

// caFingerprint is the sha256 of the first certificate, so the owner can compare it out of band.
func caFingerprint(caPEM []byte) string {
	block, _ := pem.Decode(caPEM)
	if block == nil {
		return ""
	}
	sum := sha256.Sum256(block.Bytes)
	return hex.EncodeToString(sum[:])
}

// loadK8sClient rebuilds a client from the stored credentials, for the test and read routes.
func (rt *Router) loadK8sClient(ctx context.Context, c store.K8sCluster) (*k8sconn.Client, error) {
	secrets, ok := rt.k8sSecrets()
	if !ok {
		return nil, errors.New("secrets store unavailable")
	}
	ns := store.K8sClusterSecretsKey(c.Name)
	get := func(key string) string {
		v, err := secrets.Resolve(ctx, ns, key)
		if err != nil {
			return ""
		}
		secretmask.Add(v)
		return v
	}
	creds := k8sconn.Credentials{
		Server: c.Server, ServerName: c.ServerName, Token: get(k8sSecretToken),
		CA: []byte(get(k8sSecretCA)), ClientCert: []byte(get(k8sSecretClientCert)), ClientKey: []byte(get(k8sSecretClientKey)),
	}
	if len(creds.ClientCert) == 0 {
		creds.ClientCert, creds.ClientKey = nil, nil
	}
	return rt.newK8sClient(ctx, creds)
}

func (rt *Router) handleDisconnectK8sCluster(w http.ResponseWriter, r *http.Request) {
	if !rt.k8sGate(w, r) {
		return
	}
	name := r.PathValue("name")
	prev, err := rt.k8sClusterRows.GetK8sCluster(r.Context(), name)
	if errors.Is(err, store.ErrK8sClusterNotFound) {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}
	if err == nil {
		err = rt.k8sClusterRows.DeleteK8sCluster(r.Context(), name)
	}
	if err != nil {
		rt.internalError(w, "api: delete k8s cluster failed", err)
		return
	}
	if secrets, ok := rt.k8sSecrets(); ok {
		if err := secrets.DeleteAll(r.Context(), store.K8sClusterSecretsKey(name)); err != nil {
			rt.logger.Warn("api: delete k8s credentials failed", slog.String("cluster", name))
		}
	}
	rt.auditK8sCluster(r, auditK8sClusterDisconnected, prev)
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) handleTestK8sCluster(w http.ResponseWriter, r *http.Request) {
	if !rt.k8sGate(w, r) {
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
	out := k8sTestResponse{Class: "unreachable"}
	if cl, err := rt.loadK8sClient(r.Context(), c); err == nil {
		review, rerr := cl.ReviewRules(r.Context(), "default")
		out.Class = classifyK8sError(rerr)
		out.OK = rerr == nil
		out.ClusterAdmin = rerr == nil && review.IsClusterAdmin()
	}
	_ = rt.k8sClusterRows.SetK8sClusterCheck(context.WithoutCancel(r.Context()), c.Name, out.Class, time.Now())
	rt.auditK8sCluster(r, auditK8sClusterTested, c)
	writeJSON(w, http.StatusOK, out)
}

func classifyK8sError(err error) string {
	var se *k8sconn.StatusError
	switch {
	case err == nil:
		return "ok"
	case errors.As(err, &se) && (se.Code == http.StatusUnauthorized):
		return "unauthorized"
	case errors.As(err, &se) && se.Code == http.StatusForbidden:
		return "forbidden"
	case errors.As(err, &se):
		return "error"
	}
	return "unreachable"
}

func (rt *Router) auditK8sCluster(r *http.Request, action string, c store.K8sCluster) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: k8s cluster audit id failed", slog.String("error", err.Error()))
		return
	}
	ptype, pid, _, perr := rt.callerPrincipal(r)
	if perr != nil {
		ptype, pid = auditActorSystem, "unknown"
	}
	detail, _ := json.Marshal(map[string]string{"cluster": c.Name, "server": c.Server, "auth_mode": c.AuthMode})
	entry := store.AuditEntry{
		ID: id, ActorType: ptype, ActorID: pid, ActorName: pid, Ability: AbilityRoot, Method: r.Method,
		Path: k8sClustersPath + "/" + c.Name, StatusCode: http.StatusOK, RemoteAddr: clientIP(r),
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.UserAgent()),
		Action: action, Detail: string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: save k8s cluster audit failed", slog.String("action", action))
	}
}
