package api

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/k8sconn"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	auditK8sDeployRemoved       = "k8s_deployment.removed"
	auditK8sDeployRemoveRefused = "k8s_deployment.remove_refused"
	auditK8sDeployRemoveFailed  = "k8s_deployment.remove_failed"
)

// k8sPodNameRe covers pod and container names, which are DNS-like labels.
var k8sPodNameRe = regexp.MustCompile(`^[A-Za-z0-9]([-A-Za-z0-9_.]{0,251}[A-Za-z0-9])?$`)

type k8sRemoveResponse struct {
	App       string   `json:"app"`
	Namespace string   `json:"namespace"`
	Deleted   []string `json:"deleted"`
	Absent    []string `json:"absent"`
}

// handleGetK8sDeploymentLogs returns the tail of one pod's log as text. Only pods of a
// deployment we own can be read, and the size and line counts are bounded by settings.
func (rt *Router) handleGetK8sDeploymentLogs(w http.ResponseWriter, r *http.Request) {
	if !rt.k8sGate(w, r) || !rt.k8sDeploymentsReady(w) {
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
	pod, container := r.URL.Query().Get("pod"), r.URL.Query().Get("container")
	for _, v := range []string{pod, container} {
		if v != "" && !k8sPodNameRe.MatchString(v) {
			writeError(w, http.StatusBadRequest, "invalid pod or container name")
			return
		}
	}
	maxTail := rt.runtimeSettings.Limit(ctx, runtimesettings.KeyK8sLogTailLinesMax)
	tail := maxTail
	if raw := r.URL.Query().Get("tail"); raw != "" {
		n, perr := strconv.Atoi(raw)
		if perr != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "tail must be a positive number")
			return
		}
		tail = min(n, maxTail)
	}
	d, err := rt.k8sDeploymentRows.GetK8sDeployment(ctx, c.Name, r.PathValue("app"))
	if errors.Is(err, store.ErrK8sDeploymentNotFound) {
		writeError(w, http.StatusNotFound, "deployment not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get k8s deployment failed", err)
		return
	}
	cl, err := rt.loadK8sClient(ctx, c)
	if err != nil {
		writeError(w, http.StatusBadGateway, "cluster is unreachable or its credentials cannot be loaded")
		return
	}
	instance, err := rt.k8sDeploymentRows.K8sInstanceID(ctx)
	if err != nil {
		rt.internalError(w, "api: get k8s instance id failed", err)
		return
	}
	lim := k8sconn.LoadLimits(ctx, rt.runtimeSettings)
	res, err := cl.PodLogs(ctx, d.Namespace, d.App, instance, pod, container, tail, lim.MaxBody)
	if errors.Is(err, k8sconn.ErrLogTarget) {
		writeError(w, http.StatusNotFound, "no matching pod or container for this app")
		return
	}
	if err != nil {
		writeK8sCallError(w, err, "")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Levelrail-Pod", res.Pod)
	w.Header().Set("X-Levelrail-Container", res.Container)
	if res.Truncated {
		w.Header().Set("X-Levelrail-Truncated", "true")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(res.Text))
}

// handleRemoveK8sDeployment deletes only the objects that carry our labels, never the
// namespace, then drops the record. Missing objects count as success.
func (rt *Router) handleRemoveK8sDeployment(w http.ResponseWriter, r *http.Request) {
	if !rt.k8sGate(w, r) || !rt.k8sDeploymentsReady(w) {
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
	app := r.PathValue("app")
	d, err := rt.k8sDeploymentRows.GetK8sDeployment(ctx, c.Name, app)
	if errors.Is(err, store.ErrK8sDeploymentNotFound) {
		writeError(w, http.StatusNotFound, "deployment not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get k8s deployment failed", err)
		return
	}
	audit := k8sDeployAudit{App: d.App, Namespace: d.Namespace, Image: d.Image, Host: d.Host, Port: d.Port, Replicas: d.Replicas}
	unlock := lockK8sDeploy(c.Name, d.App)
	defer unlock()
	cl, err := rt.loadK8sClient(ctx, c)
	if err != nil {
		audit.Reason = "cluster client could not be loaded"
		rt.auditK8sDeploy(r, auditK8sDeployRemoveFailed, c.Name, audit, http.StatusBadGateway)
		writeError(w, http.StatusBadGateway, "cluster is unreachable or its credentials cannot be loaded")
		return
	}
	instance, err := rt.k8sDeploymentRows.K8sInstanceID(ctx)
	if err != nil {
		rt.internalError(w, "api: get k8s instance id failed", err)
		return
	}
	res, err := cl.Remove(ctx, d.Namespace, d.App, instance, k8sPulledSecretNames(d.Objects)...)
	audit.Objects = res.Deleted
	if errors.Is(err, k8sconn.ErrNotManaged) {
		audit.Reason = "an object is not labelled as ours; nothing was deleted"
		rt.auditK8sDeploy(r, auditK8sDeployRemoveRefused, c.Name, audit, http.StatusConflict)
		writeError(w, http.StatusConflict, "an object with this name is not managed by this levelrail install; nothing was deleted")
		return
	}
	if err != nil {
		audit.Reason = "the cluster did not complete the delete"
		rt.auditK8sDeploy(r, auditK8sDeployRemoveFailed, c.Name, audit, http.StatusBadGateway)
		writeK8sCallError(w, err, "")
		return
	}
	// Audit first: the cluster already changed, whatever the row delete does next.
	rt.auditK8sDeploy(r, auditK8sDeployRemoved, c.Name, audit, http.StatusOK)
	if err := rt.k8sDeploymentRows.DeleteK8sDeployment(ctx, c.Name, d.App); err != nil {
		rt.internalError(w, "api: delete k8s deployment record failed", err)
		return
	}
	out := k8sRemoveResponse{App: d.App, Namespace: d.Namespace, Deleted: res.Deleted, Absent: res.Absent}
	if out.Deleted == nil {
		out.Deleted = []string{}
	}
	if out.Absent == nil {
		out.Absent = []string{}
	}
	writeJSON(w, http.StatusOK, out)
}
