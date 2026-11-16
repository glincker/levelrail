package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/k8sconn"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	auditK8sDeployApplied = "k8s_deployment.applied"
	auditK8sDeployRefused = "k8s_deployment.refused"
)

var (
	k8sImageRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{0,200}(:[A-Za-z0-9._-]{1,128})?(@sha256:[a-f0-9]{64})?$`)
	k8sHostRe    = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)
	k8sEnvNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
)

// K8sDeploymentRows is the store surface the deploy and status routes need.
type K8sDeploymentRows interface {
	UpsertK8sDeployment(ctx context.Context, d store.K8sDeployment) error
	GetK8sDeployment(ctx context.Context, cluster, app string) (store.K8sDeployment, error)
	ListK8sDeployments(ctx context.Context, cluster string) ([]store.K8sDeployment, error)
	DeleteK8sDeploymentsForCluster(ctx context.Context, cluster string) error
}

func k8sDeploymentStoreOf(s Store) K8sDeploymentRows {
	r, _ := s.(K8sDeploymentRows)
	return r
}

// k8sDeployRequest takes a public image only. Env values go into the Deployment as
// plain values and are never stored, logged or returned.
type k8sDeployRequest struct {
	App       string            `json:"app"`
	Namespace string            `json:"namespace,omitempty"`
	Image     string            `json:"image"`
	Port      int               `json:"port"`
	Replicas  *int              `json:"replicas,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Host      string            `json:"host,omitempty"`
	// AllowRoot lets the image run as its own user, which may be root. Without it the
	// container is forced to a non-root user id.
	AllowRoot bool   `json:"allow_root,omitempty"`
	RunAsUser *int64 `json:"run_as_user,omitempty"`
}

type k8sDeployResponse struct {
	App       string   `json:"app"`
	Namespace string   `json:"namespace"`
	Objects   []string `json:"objects"`
	AllowRoot bool     `json:"allow_root"`
}

type k8sDeploymentResource struct {
	App       string     `json:"app"`
	Namespace string     `json:"namespace"`
	Image     string     `json:"image"`
	Port      int        `json:"port"`
	Replicas  int        `json:"replicas"`
	Host      string     `json:"host,omitempty"`
	AllowRoot bool       `json:"allow_root"`
	Objects   []string   `json:"objects"`
	CreatedBy string     `json:"created_by,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type k8sDeploymentListResponse struct {
	Deployments []k8sDeploymentResource `json:"deployments"`
}

func toK8sDeploymentResource(d store.K8sDeployment) k8sDeploymentResource {
	res := k8sDeploymentResource{
		App: d.App, Namespace: d.Namespace, Image: d.Image, Port: d.Port, Replicas: d.Replicas,
		Host: d.Host, AllowRoot: d.AllowRoot, Objects: d.Objects, CreatedBy: d.CreatedBy,
	}
	if !d.CreatedAt.IsZero() {
		t := d.CreatedAt
		res.CreatedAt = &t
	}
	if !d.UpdatedAt.IsZero() {
		t := d.UpdatedAt
		res.UpdatedAt = &t
	}
	if res.Objects == nil {
		res.Objects = []string{}
	}
	return res
}

// k8sResources reads the live limits, and refuses a value that is not a plain quantity.
func (rt *Router) k8sResources(ctx context.Context) (k8sconn.Resources, bool) {
	get := func(key string) string { return rt.runtimeSettings.String(ctx, key) }
	res := k8sconn.Resources{
		CPURequest: get(runtimesettings.KeyK8sCPURequest), MemoryRequest: get(runtimesettings.KeyK8sMemoryRequest),
		CPULimit: get(runtimesettings.KeyK8sCPULimit), MemoryLimit: get(runtimesettings.KeyK8sMemoryLimit),
	}
	for _, v := range []string{res.CPURequest, res.MemoryRequest, res.CPULimit, res.MemoryLimit} {
		if !runtimesettings.ValidK8sQuantity(v) {
			return res, false
		}
	}
	return res, true
}

// publicImage accepts a registry image reference. Loopback and local registries are refused
// because a remote cluster cannot pull from them.
func publicImage(image string) bool {
	if len(image) > 255 || !k8sImageRe.MatchString(image) {
		return false
	}
	first, _, _ := strings.Cut(image, "/")
	return !strings.HasPrefix(first, "localhost") && !strings.HasPrefix(first, "127.") && !strings.HasSuffix(first, ".local")
}

func (rt *Router) handleDeployK8sApp(w http.ResponseWriter, r *http.Request) {
	if !rt.k8sGate(w, r) {
		return
	}
	if rt.k8sDeploymentRows == nil {
		writeError(w, http.StatusNotImplemented, "kubernetes deployments are not available on this store")
		return
	}
	ctx := r.Context()
	c, err := rt.k8sClusterRows.GetK8sCluster(ctx, r.PathValue("name"))
	if errors.Is(err, store.ErrK8sClusterNotFound) {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get k8s cluster failed", err)
		return
	}
	lim := k8sconn.LoadLimits(ctx, rt.runtimeSettings)
	var req k8sDeployRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, lim.MaxBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	spec, msg := rt.buildK8sSpec(ctx, req)
	if msg != "" {
		rt.auditK8sDeploy(r, auditK8sDeployRefused, c.Name, req, http.StatusBadRequest)
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	rendered, err := k8sconn.Render(spec)
	if err != nil {
		writeError(w, http.StatusBadRequest, "the app could not be rendered for the cluster")
		return
	}
	if prev, err := rt.k8sDeploymentRows.GetK8sDeployment(ctx, c.Name, spec.Name); err == nil && prev.Namespace != spec.Namespace {
		writeError(w, http.StatusConflict, "this app is already deployed to this cluster in another namespace")
		return
	}
	cl, err := rt.loadK8sClient(ctx, c)
	if err != nil {
		writeError(w, http.StatusBadGateway, "cluster is unreachable or its credentials cannot be loaded")
		return
	}
	names, err := cl.Apply(ctx, spec, rendered)
	if errors.Is(err, k8sconn.ErrNotManaged) {
		rt.auditK8sDeploy(r, auditK8sDeployRefused, c.Name, req, http.StatusConflict)
		writeError(w, http.StatusConflict, "an object with this name already exists in the namespace and is not managed by levelrail; it was left untouched")
		return
	}
	if err != nil {
		rt.auditK8sDeploy(r, auditK8sDeployRefused, c.Name, req, http.StatusBadGateway)
		writeK8sCallError(w, err)
		return
	}
	_, actor, _, _ := rt.callerPrincipal(r)
	row := store.K8sDeployment{
		Cluster: c.Name, App: spec.Name, Namespace: spec.Namespace, Image: spec.Image, Port: spec.Port,
		Replicas: spec.Replicas, Host: spec.Host, AllowRoot: req.AllowRoot, Objects: names, CreatedBy: actor,
	}
	if err := rt.k8sDeploymentRows.UpsertK8sDeployment(context.WithoutCancel(ctx), row); err != nil {
		rt.internalError(w, "api: record k8s deployment failed", err)
		return
	}
	rt.auditK8sDeploy(r, auditK8sDeployApplied, c.Name, req, http.StatusCreated)
	writeJSON(w, http.StatusCreated, k8sDeployResponse{App: spec.Name, Namespace: spec.Namespace, Objects: names, AllowRoot: req.AllowRoot})
}

// buildK8sSpec validates the request. It returns a message for the caller on refusal.
//
// Root policy: unless allow_root is true the container is forced to a non-root user id
// (run_as_user, else the k8s.run_as_user setting), so a public image that needs root fails
// to start instead of running as root. run_as_user 0 needs allow_root.
func (rt *Router) buildK8sSpec(ctx context.Context, req k8sDeployRequest) (k8sconn.AppSpec, string) {
	set := rt.runtimeSettings
	ns := req.Namespace
	if ns == "" {
		ns = set.String(ctx, runtimesettings.KeyK8sDefaultNamespace)
	}
	replicas := 1
	if req.Replicas != nil {
		replicas = *req.Replicas
	}
	switch {
	case !k8sClusterNameRe.MatchString(req.App):
		return k8sconn.AppSpec{}, "app must be lowercase letters, digits and dashes, up to 63 characters"
	case !k8sClusterNameRe.MatchString(ns):
		return k8sconn.AppSpec{}, "namespace must be lowercase letters, digits and dashes, up to 63 characters"
	case !publicImage(req.Image):
		return k8sconn.AppSpec{}, "image must be a public registry image such as nginx:1.27"
	case req.Port < 1 || req.Port > 65535:
		return k8sconn.AppSpec{}, "port must be between 1 and 65535"
	case replicas < 0 || replicas > set.Limit(ctx, runtimesettings.KeyK8sMaxReplicas):
		return k8sconn.AppSpec{}, "replicas is over the limit set by k8s.max_replicas"
	case req.Host != "" && (len(req.Host) > 253 || !k8sHostRe.MatchString(req.Host)):
		return k8sconn.AppSpec{}, "host must be a lowercase DNS name"
	case len(req.Env) > set.Limit(ctx, runtimesettings.KeyK8sMaxEnvVars):
		return k8sconn.AppSpec{}, "env has more variables than k8s.max_env_vars allows"
	case req.RunAsUser != nil && *req.RunAsUser < 0:
		return k8sconn.AppSpec{}, "run_as_user must not be negative"
	case req.RunAsUser != nil && *req.RunAsUser == 0 && !req.AllowRoot:
		return k8sconn.AppSpec{}, "run_as_user 0 is root; set allow_root to true to run as root"
	}
	for k := range req.Env {
		if !k8sEnvNameRe.MatchString(k) {
			return k8sconn.AppSpec{}, "env names must be letters, digits and underscores"
		}
	}
	res, ok := rt.k8sResources(ctx)
	if !ok {
		return k8sconn.AppSpec{}, "the k8s resource request and limit settings are not valid quantities"
	}
	spec := k8sconn.AppSpec{
		Name: req.App, Namespace: ns, Image: req.Image, Port: req.Port, Replicas: replicas,
		Env: req.Env, Host: req.Host, Resources: res,
	}
	switch {
	case req.RunAsUser != nil && *req.RunAsUser > 0:
		spec.RunAsUser = req.RunAsUser
	case !req.AllowRoot:
		uid := int64(set.Limit(ctx, runtimesettings.KeyK8sRunAsUser))
		spec.RunAsUser = &uid
	}
	return spec, ""
}

func (rt *Router) handleListK8sDeployments(w http.ResponseWriter, r *http.Request) {
	if !rt.k8sGate(w, r) || !rt.k8sDeploymentsReady(w) {
		return
	}
	name := r.PathValue("name")
	if _, err := rt.k8sClusterRows.GetK8sCluster(r.Context(), name); errors.Is(err, store.ErrK8sClusterNotFound) {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: get k8s cluster failed", err)
		return
	}
	list, err := rt.k8sDeploymentRows.ListK8sDeployments(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: list k8s deployments failed", err)
		return
	}
	out := k8sDeploymentListResponse{Deployments: make([]k8sDeploymentResource, 0, len(list))}
	for _, d := range list {
		out.Deployments = append(out.Deployments, toK8sDeploymentResource(d))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) k8sDeploymentsReady(w http.ResponseWriter) bool {
	if rt.k8sDeploymentRows == nil {
		writeError(w, http.StatusNotImplemented, "kubernetes deployments are not available on this store")
		return false
	}
	return true
}

// auditK8sDeploy records names and the image, never env values or credentials.
func (rt *Router) auditK8sDeploy(r *http.Request, action, cluster string, req k8sDeployRequest, status int) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: k8s deploy audit id failed", slog.String("error", err.Error()))
		return
	}
	ptype, pid, _, perr := rt.callerPrincipal(r)
	if perr != nil {
		ptype, pid = auditActorSystem, "unknown"
	}
	ns := req.Namespace
	detail, _ := json.Marshal(map[string]any{
		"cluster": cluster, "app": req.App, "namespace": ns, "image": req.Image, "allow_root": req.AllowRoot,
	})
	entry := store.AuditEntry{
		ID: id, ActorType: ptype, ActorID: pid, ActorName: pid, Ability: AbilityRoot, Method: r.Method,
		Path: k8sClustersPath + "/" + cluster + "/deployments", StatusCode: status, RemoteAddr: clientIP(r),
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.UserAgent()),
		Action: action, Detail: string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: save k8s deploy audit failed", slog.String("action", action))
	}
}
