package api

import (
	"errors"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

type k8sConditionRow struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type k8sPodRow struct {
	Name   string `json:"name"`
	Phase  string `json:"phase"`
	Reason string `json:"reason,omitempty"`
}

type k8sDeploymentStatusResponse struct {
	App            string            `json:"app"`
	Namespace      string            `json:"namespace"`
	Found          bool              `json:"found"`
	Ready          bool              `json:"ready"`
	Replicas       int               `json:"replicas"`
	ReadyReplicas  int               `json:"ready_replicas"`
	Updated        int               `json:"updated_replicas"`
	Conditions     []k8sConditionRow `json:"conditions"`
	Pods           []k8sPodRow       `json:"pods"`
	IngressAddress string            `json:"ingress_address,omitempty"`
	Reason         string            `json:"reason,omitempty"`
}

func (rt *Router) handleGetK8sDeploymentStatus(w http.ResponseWriter, r *http.Request) {
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
	st, err := cl.DeploymentStatus(ctx, d.Namespace, d.App)
	if err != nil {
		writeK8sCallError(w, err)
		return
	}
	out := k8sDeploymentStatusResponse{
		App: d.App, Namespace: d.Namespace, Found: st.Found, Replicas: st.Replicas, ReadyReplicas: st.ReadyReplicas,
		Updated: st.UpdatedReplicas, IngressAddress: st.IngressAddress, Reason: st.Reason,
		Conditions: []k8sConditionRow{}, Pods: []k8sPodRow{},
	}
	out.Ready = st.Found && st.ReadyReplicas >= d.Replicas && d.Replicas > 0
	if !st.Found {
		out.Reason = "the objects are gone from the cluster"
	}
	for _, cd := range st.Conditions {
		out.Conditions = append(out.Conditions, k8sConditionRow{Type: cd.Type, Status: cd.Status, Reason: cd.Reason, Message: truncateK8sMessage(cd.Message)})
	}
	for _, p := range st.Pods {
		out.Pods = append(out.Pods, k8sPodRow{Name: p.Name, Phase: p.Phase, Reason: p.Reason})
	}
	writeJSON(w, http.StatusOK, out)
}

// truncateK8sMessage keeps cluster-authored text short so a status row stays small.
func truncateK8sMessage(s string) string {
	const limit = 300
	if len(s) > limit {
		return s[:limit]
	}
	return s
}
