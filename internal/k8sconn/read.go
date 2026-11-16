package k8sconn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Node is the trimmed view of a core/v1 Node.
type Node struct {
	Name    string
	Ready   bool
	Roles   []string
	Kubelet string
}

// Namespace is the trimmed view of a core/v1 Namespace.
type Namespace struct {
	Name  string
	Phase string
}

// ResourceRule mirrors authorization/v1 ResourceRule.
type ResourceRule struct {
	Verbs     []string `json:"verbs"`
	APIGroups []string `json:"apiGroups"`
	Resources []string `json:"resources"`
}

// RulesReview is the decoded SelfSubjectRulesReview status.
type RulesReview struct {
	Rules      []ResourceRule
	Incomplete bool
}

type nodeList struct {
	Items []struct {
		Metadata struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Status struct {
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
			NodeInfo struct {
				KubeletVersion string `json:"kubeletVersion"`
			} `json:"nodeInfo"`
		} `json:"status"`
	} `json:"items"`
}

// ListNodes returns every node with readiness and role labels.
func (c *Client) ListNodes(ctx context.Context) ([]Node, error) {
	data, err := c.Do(ctx, "GET", "/api/v1/nodes", nil)
	if err != nil {
		return nil, err
	}
	var l nodeList
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("k8sconn: decode nodes: %w", err)
	}
	out := make([]Node, 0, len(l.Items))
	const rolePrefix = "node-role.kubernetes.io/"
	for _, it := range l.Items {
		n := Node{Name: it.Metadata.Name, Kubelet: it.Status.NodeInfo.KubeletVersion}
		for k := range it.Metadata.Labels {
			if r, ok := strings.CutPrefix(k, rolePrefix); ok && r != "" {
				n.Roles = append(n.Roles, r)
			}
		}
		for _, cond := range it.Status.Conditions {
			if cond.Type == "Ready" {
				n.Ready = cond.Status == "True"
			}
		}
		out = append(out, n)
	}
	return out, nil
}

// ListNamespaces returns every namespace with its phase.
func (c *Client) ListNamespaces(ctx context.Context) ([]Namespace, error) {
	data, err := c.Do(ctx, "GET", "/api/v1/namespaces", nil)
	if err != nil {
		return nil, err
	}
	var l struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Phase string `json:"phase"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("k8sconn: decode namespaces: %w", err)
	}
	out := make([]Namespace, 0, len(l.Items))
	for _, it := range l.Items {
		out = append(out, Namespace{Name: it.Metadata.Name, Phase: it.Status.Phase})
	}
	return out, nil
}

// ReviewRules asks the API server what the credential may do in a namespace.
func (c *Client) ReviewRules(ctx context.Context, namespace string) (RulesReview, error) {
	body, err := json.Marshal(map[string]any{
		"apiVersion": "authorization.k8s.io/v1",
		"kind":       "SelfSubjectRulesReview",
		"spec":       map[string]string{"namespace": namespace},
	})
	if err != nil {
		return RulesReview{}, err
	}
	data, err := c.Do(ctx, "POST", "/apis/authorization.k8s.io/v1/selfsubjectrulesreviews", body)
	if err != nil {
		return RulesReview{}, err
	}
	var resp struct {
		Status struct {
			ResourceRules []ResourceRule `json:"resourceRules"`
			Incomplete    bool           `json:"incomplete"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return RulesReview{}, fmt.Errorf("k8sconn: decode rules review: %w", err)
	}
	return RulesReview{Rules: resp.Status.ResourceRules, Incomplete: resp.Status.Incomplete}, nil
}

// IsClusterAdmin reports a wildcard rule over every group, resource and verb.
func (r RulesReview) IsClusterAdmin() bool {
	has := func(s []string, v string) bool {
		for _, x := range s {
			if x == v {
				return true
			}
		}
		return false
	}
	for _, rule := range r.Rules {
		if has(rule.Verbs, "*") && has(rule.APIGroups, "*") && has(rule.Resources, "*") {
			return true
		}
	}
	return false
}

// ClusterAdminRefusal explains the default refusal and the namespaced alternative.
const ClusterAdminRefusal = "this credential is effectively cluster-admin (wildcard verbs on all resources and api groups) and is refused; " +
	"create a namespaced Role and a limited service account with `levelrail clusters rbac --namespace NS`, then connect that token. " +
	"To accept the risk, set allow_cluster_admin on the request (--allow-cluster-admin) and turn on the runtime setting k8s.allow_cluster_admin"

// ErrAdminCheckUnverified means the rules review failed or was incomplete, so the credential cannot be proven non-admin.
var ErrAdminCheckUnverified = errors.New("could not verify the credential is not cluster-admin")

// CredentialIsClusterAdmin asks the API server whether this credential holds a wildcard rule.
// It fails closed: an errored or incomplete review returns ErrAdminCheckUnverified.
func (c *Client) CredentialIsClusterAdmin(ctx context.Context) (bool, error) {
	review, err := c.ReviewRules(ctx, "default")
	if err != nil {
		return false, ErrAdminCheckUnverified
	}
	if review.IsClusterAdmin() {
		return true, nil
	}
	if review.Incomplete {
		return false, ErrAdminCheckUnverified
	}
	return false, nil
}
