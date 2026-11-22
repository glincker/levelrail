package apiclient

import (
	"context"
	"net/http"
	"time"
)

// K8sClusterResource is a connected cluster; the API never returns credentials.
type K8sClusterResource struct {
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

// ConnectK8sClusterRequest carries a kubeconfig, or a server with token and CA.
type ConnectK8sClusterRequest struct {
	Name       string `json:"name"`
	Kubeconfig string `json:"kubeconfig,omitempty"`
	Context    string `json:"context,omitempty"`
	Server     string `json:"server,omitempty"`
	Token      string `json:"token,omitempty"`
	CA         string `json:"ca,omitempty"`
	// AllowClusterAdmin accepts a cluster-admin credential when the server setting k8s.allow_cluster_admin is on.
	AllowClusterAdmin bool `json:"allow_cluster_admin,omitempty"`
}

// K8sClusterTestResult is the read-only connection check outcome.
type K8sClusterTestResult struct {
	OK           bool   `json:"ok"`
	Class        string `json:"class"`
	ClusterAdmin bool   `json:"cluster_admin,omitempty"`
}

// K8sNodeRow is one node as the control plane reports it.
type K8sNodeRow struct {
	Name    string   `json:"name"`
	Ready   bool     `json:"ready"`
	Roles   []string `json:"roles"`
	Kubelet string   `json:"kubelet_version"`
}

// K8sNamespaceRow is one namespace and its phase.
type K8sNamespaceRow struct {
	Name  string `json:"name"`
	Phase string `json:"phase"`
}

func k8sClusterPath(name string) string {
	return "/api/v1/clusters/" + PathEscape(name)
}

// ListK8sClusters calls GET /api/v1/clusters.
func (c *Client) ListK8sClusters(ctx context.Context) ([]K8sClusterResource, error) {
	var out struct {
		Clusters []K8sClusterResource `json:"clusters"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/clusters", nil, &out)
	return out.Clusters, err
}

// ConnectK8sCluster calls POST /api/v1/clusters.
func (c *Client) ConnectK8sCluster(ctx context.Context, req ConnectK8sClusterRequest) (K8sClusterResource, error) {
	var out K8sClusterResource
	err := c.do(ctx, http.MethodPost, "/api/v1/clusters", req, &out)
	return out, err
}

// GetK8sCluster calls GET /api/v1/clusters/{name}.
func (c *Client) GetK8sCluster(ctx context.Context, name string) (K8sClusterResource, error) {
	return getJSON[K8sClusterResource](ctx, c, k8sClusterPath(name))
}

// TestK8sCluster calls POST /api/v1/clusters/{name}/test.
func (c *Client) TestK8sCluster(ctx context.Context, name string) (K8sClusterTestResult, error) {
	var out K8sClusterTestResult
	err := c.do(ctx, http.MethodPost, k8sClusterPath(name)+"/test", nil, &out)
	return out, err
}

// ListK8sNodes calls GET /api/v1/clusters/{name}/nodes.
func (c *Client) ListK8sNodes(ctx context.Context, name string) ([]K8sNodeRow, error) {
	var out struct {
		Nodes []K8sNodeRow `json:"nodes"`
	}
	err := c.do(ctx, http.MethodGet, k8sClusterPath(name)+"/nodes", nil, &out)
	return out.Nodes, err
}

// ListK8sNamespaces calls GET /api/v1/clusters/{name}/namespaces.
func (c *Client) ListK8sNamespaces(ctx context.Context, name string) ([]K8sNamespaceRow, error) {
	var out struct {
		Namespaces []K8sNamespaceRow `json:"namespaces"`
	}
	err := c.do(ctx, http.MethodGet, k8sClusterPath(name)+"/namespaces", nil, &out)
	return out.Namespaces, err
}

// DisconnectK8sCluster calls DELETE /api/v1/clusters/{name}.
func (c *Client) DisconnectK8sCluster(ctx context.Context, name string) error {
	return c.del(ctx, k8sClusterPath(name))
}

// K8sDeployRequest is the body of POST /clusters/{name}/deployments.
type K8sDeployRequest struct {
	App       string            `json:"app"`
	Namespace string            `json:"namespace,omitempty"`
	Image     string            `json:"image"`
	Port      int               `json:"port"`
	Replicas  *int              `json:"replicas,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Host      string            `json:"host,omitempty"`
	AllowRoot bool              `json:"allow_root,omitempty"`
	// PullSecret names a stored registry credential for a private image.
	PullSecret string `json:"pull_secret,omitempty"`
}

// K8sDeployResult lists the objects the deploy applied.
type K8sDeployResult struct {
	App       string   `json:"app"`
	Namespace string   `json:"namespace"`
	Objects   []string `json:"objects"`
	AllowRoot bool     `json:"allow_root"`
}

// K8sDeploymentRow is one recorded deployment; env values are never returned.
type K8sDeploymentRow struct {
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

// K8sConditionRow is one workload condition.
type K8sConditionRow struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// K8sPodRow is one pod of the workload.
type K8sPodRow struct {
	Name    string `json:"name"`
	Phase   string `json:"phase"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// K8sDeploymentStatus is the live rollout state of one app.
type K8sDeploymentStatus struct {
	App            string            `json:"app"`
	Namespace      string            `json:"namespace"`
	Found          bool              `json:"found"`
	Ready          bool              `json:"ready"`
	Replicas       int               `json:"replicas"`
	ReadyReplicas  int               `json:"ready_replicas"`
	Updated        int               `json:"updated_replicas"`
	Conditions     []K8sConditionRow `json:"conditions"`
	Pods           []K8sPodRow       `json:"pods"`
	IngressAddress string            `json:"ingress_address,omitempty"`
	Reason         string            `json:"reason,omitempty"`
}

// DeployK8sApp calls POST /api/v1/clusters/{name}/deployments.
func (c *Client) DeployK8sApp(ctx context.Context, name string, req K8sDeployRequest) (K8sDeployResult, error) {
	var out K8sDeployResult
	err := c.do(ctx, http.MethodPost, k8sClusterPath(name)+"/deployments", req, &out)
	return out, err
}

// ListK8sDeployments calls GET /api/v1/clusters/{name}/deployments.
func (c *Client) ListK8sDeployments(ctx context.Context, name string) ([]K8sDeploymentRow, error) {
	var out struct {
		Deployments []K8sDeploymentRow `json:"deployments"`
	}
	err := c.do(ctx, http.MethodGet, k8sClusterPath(name)+"/deployments", nil, &out)
	return out.Deployments, err
}

// GetK8sDeploymentStatus calls GET /api/v1/clusters/{name}/deployments/{app}.
func (c *Client) GetK8sDeploymentStatus(ctx context.Context, name, app string) (K8sDeploymentStatus, error) {
	return getJSON[K8sDeploymentStatus](ctx, c, k8sClusterPath(name)+"/deployments/"+PathEscape(app))
}
