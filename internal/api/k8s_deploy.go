package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/k8sconn"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	auditK8sDeployApplied = "k8s_deployment.applied"
	auditK8sDeployRefused = "k8s_deployment.refused"
	auditK8sDeployFailed  = "k8s_deployment.failed"
	auditK8sDeployPartial = "k8s_deployment.partial"
)

var (
	// The optional first group is a registry host with a port (host:5000/img).
	k8sImageRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?:[0-9]{1,5}/)?[a-z0-9][a-z0-9._/-]{0,200}(:[A-Za-z0-9._-]{1,128})?(@sha256:[a-f0-9]{64})?$`)
	// App names become Service names, which must start with a letter.
	k8sAppNameRe = regexp.MustCompile(`^[a-z]([-a-z0-9]{0,61}[a-z0-9])?$`)
	k8sHostRe    = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)
	k8sEnvNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
)

// K8sDeploymentRows is the store surface the deploy and status routes need.
type K8sDeploymentRows interface {
	UpsertK8sDeployment(ctx context.Context, d store.K8sDeployment) error
	GetK8sDeployment(ctx context.Context, cluster, app string) (store.K8sDeployment, error)
	ListK8sDeployments(ctx context.Context, cluster string) ([]store.K8sDeployment, error)
	DeleteK8sDeploymentsForCluster(ctx context.Context, cluster string) error
	K8sInstanceID(ctx context.Context) (string, error)
	ClaimK8sDeploy(ctx context.Context, cluster, app string, staleAfter time.Duration) (bool, error)
	ReleaseK8sDeploy(ctx context.Context, cluster, app string) error
}

// k8sDeployLocks serialises deploys of one app on one cluster inside this process. The DB
// claim covers a second process or a crash.
var k8sDeployLocks sync.Map

func lockK8sDeploy(cluster, app string) func() {
	v, _ := k8sDeployLocks.LoadOrStore(cluster+"/"+app, &sync.Mutex{})
	mu, _ := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
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
	// SkipHostCheck proceeds when the cluster refuses to list Ingresses. It works only while
	// k8s.allow_unverified_hosts is on.
	SkipHostCheck bool `json:"skip_host_check,omitempty"`
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
		rt.auditK8sDeploy(r, auditK8sDeployRefused, c.Name, rt.k8sAuditFromRequest(ctx, req, msg), http.StatusBadRequest)
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	detail := k8sAuditFromSpec(spec, req.AllowRoot)
	instance, err := rt.k8sDeploymentRows.K8sInstanceID(ctx)
	if err != nil {
		detail.Reason = "instance id unavailable"
		rt.auditK8sDeploy(r, auditK8sDeployFailed, c.Name, detail, http.StatusInternalServerError)
		rt.internalError(w, "api: k8s instance id failed", err)
		return
	}
	spec.InstanceID = instance
	rendered, err := k8sconn.Render(spec)
	if err != nil {
		detail.Reason = "render failed"
		rt.auditK8sDeploy(r, auditK8sDeployFailed, c.Name, detail, http.StatusBadRequest)
		writeError(w, http.StatusBadRequest, "the app could not be rendered for the cluster")
		return
	}
	unlock := lockK8sDeploy(c.Name, spec.Name)
	defer unlock()
	claimTTL := 10 * lim.Timeout
	got, err := rt.k8sDeploymentRows.ClaimK8sDeploy(ctx, c.Name, spec.Name, claimTTL)
	if err != nil {
		rt.internalError(w, "api: claim k8s deploy failed", err)
		return
	}
	if !got {
		detail.Reason = "another deploy of this app is running"
		rt.auditK8sDeploy(r, auditK8sDeployRefused, c.Name, detail, http.StatusConflict)
		writeError(w, http.StatusConflict, "another deploy of this app on this cluster is already running")
		return
	}
	defer func() { _ = rt.k8sDeploymentRows.ReleaseK8sDeploy(context.WithoutCancel(ctx), c.Name, spec.Name) }()
	if prev, err := rt.k8sDeploymentRows.GetK8sDeployment(ctx, c.Name, spec.Name); err == nil && prev.Namespace != spec.Namespace {
		detail.Reason = "app is deployed in another namespace"
		rt.auditK8sDeploy(r, auditK8sDeployRefused, c.Name, detail, http.StatusConflict)
		writeError(w, http.StatusConflict, "this app is already deployed to this cluster in another namespace")
		return
	}
	cl, err := rt.loadK8sClient(ctx, c)
	if err != nil {
		detail.Reason = "cluster client could not be loaded"
		rt.auditK8sDeploy(r, auditK8sDeployFailed, c.Name, detail, http.StatusBadGateway)
		writeError(w, http.StatusBadGateway, "cluster is unreachable or its credentials cannot be loaded")
		return
	}
	names, err := cl.Apply(ctx, spec, rendered)
	detail.Objects = names
	if err != nil {
		rt.failK8sApply(w, r, c.Name, detail, err)
		return
	}
	// Audit first: the cluster already changed, whatever the row write does next.
	rt.auditK8sDeploy(r, auditK8sDeployApplied, c.Name, detail, http.StatusCreated)
	_, actor, _, _ := rt.callerPrincipal(r)
	row := store.K8sDeployment{
		Cluster: c.Name, App: spec.Name, Namespace: spec.Namespace, Image: spec.Image, Port: spec.Port,
		Replicas: spec.Replicas, Host: spec.Host, AllowRoot: req.AllowRoot, Objects: names, CreatedBy: actor,
	}
	if err := rt.k8sDeploymentRows.UpsertK8sDeployment(context.WithoutCancel(ctx), row); err != nil {
		detail.Reason = "the cluster changed but the deployment record was not saved"
		rt.auditK8sDeploy(r, auditK8sDeployFailed, c.Name, detail, http.StatusInternalServerError)
		rt.internalError(w, "api: record k8s deployment failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, k8sDeployResponse{App: spec.Name, Namespace: spec.Namespace, Objects: names, AllowRoot: req.AllowRoot})
}

// failK8sApply audits and answers a failed Apply. A partial apply is logged as partial.
func (rt *Router) failK8sApply(w http.ResponseWriter, r *http.Request, cluster string, d k8sDeployAudit, err error) {
	status, msg := http.StatusBadGateway, ""
	switch {
	case errors.Is(err, k8sconn.ErrNamespaceNotManaged):
		status, msg = http.StatusConflict, "the namespace already exists and is not managed by levelrail; add it to k8s.allowed_namespaces to use it"
	case errors.Is(err, k8sconn.ErrHostTaken):
		status, msg = http.StatusConflict, "the host is already used by an ingress that levelrail does not own; nothing was changed"
	case errors.Is(err, k8sconn.ErrHostUnverified):
		status, msg = http.StatusConflict, "the cluster did not allow listing ingresses, so the host cannot be verified; set skip_host_check while k8s.allow_unverified_hosts is on to override"
	case errors.Is(err, k8sconn.ErrNotManaged):
		status, msg = http.StatusConflict, "an object with this name already exists in the namespace and is not managed by levelrail; it was not merged into"
	}
	action := auditK8sDeployFailed
	if status == http.StatusConflict {
		action = auditK8sDeployRefused
	}
	if len(d.Objects) > 0 {
		action = auditK8sDeployPartial
		d.Reason = "some objects were applied before the failure"
		rt.recordPartialK8s(r, cluster, d)
	}
	rt.auditK8sDeploy(r, action, cluster, d, status)
	if msg != "" {
		writeError(w, status, msg)
		return
	}
	writeK8sCallError(w, err)
}

// recordPartialK8s keeps the row so status and removal see what was applied.
func (rt *Router) recordPartialK8s(r *http.Request, cluster string, d k8sDeployAudit) {
	_, actor, _, _ := rt.callerPrincipal(r)
	row := store.K8sDeployment{
		Cluster: cluster, App: d.App, Namespace: d.Namespace, Image: d.Image, Port: d.Port, Replicas: d.Replicas,
		Host: d.Host, AllowRoot: d.AllowRoot, Objects: d.Objects, CreatedBy: actor,
	}
	if err := rt.k8sDeploymentRows.UpsertK8sDeployment(context.WithoutCancel(r.Context()), row); err != nil {
		rt.logger.Warn("api: record partial k8s deployment failed", slog.String("cluster", cluster))
	}
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
	case !k8sAppNameRe.MatchString(req.App):
		return k8sconn.AppSpec{}, "app must start with a lowercase letter, then letters, digits and dashes, up to 63 characters"
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
	case req.SkipHostCheck && !set.Bool(ctx, runtimesettings.KeyK8sAllowUnverifiedHost):
		return k8sconn.AppSpec{}, "skip_host_check needs the k8s.allow_unverified_hosts setting"
	case req.RunAsUser != nil && *req.RunAsUser < 0:
		return k8sconn.AppSpec{}, "run_as_user must not be negative"
	case req.RunAsUser != nil && *req.RunAsUser == 0 && !req.AllowRoot:
		return k8sconn.AppSpec{}, "run_as_user 0 is root; set allow_root to true to run as root"
	}
	maxVal := set.Limit(ctx, runtimesettings.KeyK8sMaxEnvValueBytes)
	for k, v := range req.Env {
		if !k8sEnvNameRe.MatchString(k) {
			return k8sconn.AppSpec{}, "env names must be letters, digits and underscores"
		}
		if len(v) > maxVal {
			return k8sconn.AppSpec{}, "an env value is longer than k8s.max_env_value_bytes allows"
		}
	}
	res, ok := rt.k8sResources(ctx)
	if !ok {
		return k8sconn.AppSpec{}, "the k8s resource request and limit settings are not valid quantities"
	}
	spec := k8sconn.AppSpec{
		Name: req.App, Namespace: ns, Image: req.Image, Port: req.Port, Replicas: replicas,
		Env: req.Env, Host: req.Host, Resources: res, AllowUnverifiedHost: req.SkipHostCheck,
		AllowUnmanagedNamespace: ns == set.String(ctx, runtimesettings.KeyK8sDefaultNamespace) || k8sNamespaceAllowed(set.String(ctx, runtimesettings.KeyK8sAllowedNamespaces), ns),
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

func k8sNamespaceAllowed(list, ns string) bool {
	for _, a := range runtimesettings.ParseK8sNamespaces(list) {
		if a == ns {
			return true
		}
	}
	return false
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

// k8sDeployAudit is what a deploy audit entry carries: names and values, never env values.
type k8sDeployAudit struct {
	App, Namespace, Image, Host string
	Port, Replicas              int
	RunAsUser                   *int64
	AllowRoot                   bool
	Objects                     []string
	Reason                      string
}

// k8sAuditFromRequest resolves the effective namespace and replicas for a request that
// failed validation, so the entry still shows what would have been applied.
func (rt *Router) k8sAuditFromRequest(ctx context.Context, req k8sDeployRequest, reason string) k8sDeployAudit {
	ns := req.Namespace
	if ns == "" {
		ns = rt.runtimeSettings.String(ctx, runtimesettings.KeyK8sDefaultNamespace)
	}
	replicas := 1
	if req.Replicas != nil {
		replicas = *req.Replicas
	}
	return k8sDeployAudit{
		App: req.App, Namespace: ns, Image: req.Image, Host: req.Host, Port: req.Port, Replicas: replicas,
		RunAsUser: req.RunAsUser, AllowRoot: req.AllowRoot, Reason: reason,
	}
}

func k8sAuditFromSpec(spec k8sconn.AppSpec, allowRoot bool) k8sDeployAudit {
	return k8sDeployAudit{
		App: spec.Name, Namespace: spec.Namespace, Image: spec.Image, Host: spec.Host, Port: spec.Port,
		Replicas: spec.Replicas, RunAsUser: spec.RunAsUser, AllowRoot: allowRoot,
	}
}

func (rt *Router) auditK8sDeploy(r *http.Request, action, cluster string, d k8sDeployAudit, status int) {
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
	detail, _ := json.Marshal(map[string]any{
		"cluster": cluster, "app": d.App, "namespace": d.Namespace, "image": d.Image, "host": d.Host,
		"port": d.Port, "replicas": d.Replicas, "run_as_user": d.RunAsUser, "allow_root": d.AllowRoot,
		"objects": d.Objects, "reason": d.Reason,
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
