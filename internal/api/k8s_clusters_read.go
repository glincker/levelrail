package api

import (
	"errors"
	"net/http"

	"github.com/glincker/levelrail/internal/k8sconn"
	"github.com/glincker/levelrail/internal/store"
)

type k8sNodeRow struct {
	Name    string   `json:"name"`
	Ready   bool     `json:"ready"`
	Roles   []string `json:"roles"`
	Kubelet string   `json:"kubelet_version"`
}

type k8sNamespaceRow struct {
	Name  string `json:"name"`
	Phase string `json:"phase"`
}

// k8sReadClient resolves the named cluster to a client, or writes the refusal.
func (rt *Router) k8sReadClient(w http.ResponseWriter, r *http.Request) (*k8sconn.Client, bool) {
	if !rt.k8sGate(w, r) {
		return nil, false
	}
	c, err := rt.k8sClusterRows.GetK8sCluster(r.Context(), r.PathValue("name"))
	if errors.Is(err, store.ErrK8sClusterNotFound) {
		writeError(w, http.StatusNotFound, "cluster not found")
		return nil, false
	}
	if err != nil {
		rt.internalError(w, "api: get k8s cluster failed", err)
		return nil, false
	}
	cl, err := rt.loadK8sClient(r.Context(), c)
	if err != nil {
		writeError(w, http.StatusBadGateway, "cluster is unreachable or its credentials cannot be loaded")
		return nil, false
	}
	return cl, true
}

// writeK8sCallError maps a cluster call failure to a status without echoing the server body.
// need names the permission the call required, empty when it is not a cluster-scope read.
func writeK8sCallError(w http.ResponseWriter, err error, need string) {
	switch classifyK8sError(err) {
	case "unauthorized", "forbidden":
		if need != "" {
			writeError(w, http.StatusBadGateway, "the cluster refused the stored credential: it lacks the cluster-scope permission "+need+"; the namespaced role from clusters rbac does not grant it, add the read-only ClusterRole from clusters rbac --cluster-read")
			return
		}
		writeError(w, http.StatusBadGateway, "the cluster refused the stored credential for this read")
	default:
		writeError(w, http.StatusBadGateway, "the cluster did not answer the request")
	}
}

func (rt *Router) handleListK8sNodes(w http.ResponseWriter, r *http.Request) {
	cl, ok := rt.k8sReadClient(w, r)
	if !ok {
		return
	}
	nodes, err := cl.ListNodes(r.Context())
	if err != nil {
		writeK8sCallError(w, err, "list nodes")
		return
	}
	out := make([]k8sNodeRow, 0, len(nodes))
	for _, n := range nodes {
		roles := n.Roles
		if roles == nil {
			roles = []string{}
		}
		out = append(out, k8sNodeRow{Name: n.Name, Ready: n.Ready, Roles: roles, Kubelet: n.Kubelet})
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": out})
}

func (rt *Router) handleListK8sNamespaces(w http.ResponseWriter, r *http.Request) {
	cl, ok := rt.k8sReadClient(w, r)
	if !ok {
		return
	}
	list, err := cl.ListNamespaces(r.Context())
	if err != nil {
		writeK8sCallError(w, err, "list namespaces")
		return
	}
	out := make([]k8sNamespaceRow, 0, len(list))
	for _, n := range list {
		out = append(out, k8sNamespaceRow{Name: n.Name, Phase: n.Phase})
	}
	writeJSON(w, http.StatusOK, map[string]any{"namespaces": out})
}
