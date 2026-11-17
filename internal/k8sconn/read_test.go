package k8sconn

import (
	"context"
	"testing"
)

func TestReadCalls(t *testing.T) {
	c := newFake(t, map[string]string{
		"GET /api/v1/nodes": `{"items":[{"metadata":{"name":"n1","labels":{"node-role.kubernetes.io/control-plane":""}},
			"status":{"conditions":[{"type":"Ready","status":"True"}],"nodeInfo":{"kubeletVersion":"v1.30.0"}}}]}`,
		"GET /api/v1/namespaces": `{"items":[{"metadata":{"name":"default"},"status":{"phase":"Active"}}]}`,
		"POST /apis/authorization.k8s.io/v1/selfsubjectrulesreviews": `{"status":{"resourceRules":[
			{"verbs":["*"],"apiGroups":["*"],"resources":["*"]}],"incomplete":false}}`,
	})
	ctx := context.Background()

	nodes, err := c.ListNodes(ctx)
	if err != nil || len(nodes) != 1 || !nodes[0].Ready || nodes[0].Roles[0] != "control-plane" || nodes[0].Kubelet != "v1.30.0" {
		t.Fatalf("nodes: %+v %v", nodes, err)
	}
	ns, err := c.ListNamespaces(ctx)
	if err != nil || len(ns) != 1 || ns[0].Name != "default" || ns[0].Phase != "Active" {
		t.Fatalf("namespaces: %+v %v", ns, err)
	}
	rr, err := c.ReviewRules(ctx, "default")
	if err != nil || len(rr.Rules) != 1 || !rr.IsClusterAdmin() {
		t.Fatalf("review: %+v %v", rr, err)
	}
	scoped := RulesReview{Rules: []ResourceRule{{Verbs: []string{"get"}, APIGroups: []string{"apps"}, Resources: []string{"deployments"}}}}
	if scoped.IsClusterAdmin() {
		t.Error("scoped role flagged as cluster-admin")
	}
}
