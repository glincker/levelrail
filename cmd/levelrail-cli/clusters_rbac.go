package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
)

var clusterRBACNameRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)

// Namespace-scoped on purpose: no ClusterRole, so a leaked token cannot reach other namespaces.
func clustersRBACYAML(namespace, account string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: %[2]s
  namespace: %[1]s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: %[2]s
  namespace: %[1]s
rules:
  - apiGroups: ["", "apps", "networking.k8s.io"]
    resources: ["deployments", "services", "ingresses", "pods", "pods/log", "secrets"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: %[2]s
  namespace: %[1]s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: %[2]s
subjects:
  - kind: ServiceAccount
    name: %[2]s
    namespace: %[1]s
`, namespace, account)
}

// Optional and separate so the default stays namespace-only: nodes, namespaces and the ingress host check are cluster scoped.
func clustersClusterReadYAML(namespace, account string) string {
	return fmt.Sprintf(`---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: %[2]s-cluster-read
rules:
  - apiGroups: [""]
    resources: ["nodes", "namespaces"]
    verbs: ["get", "list"]
  # Lets a deploy with a host check that no other ingress claims it.
  - apiGroups: ["networking.k8s.io"]
    resources: ["ingresses"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: %[2]s-cluster-read
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: %[2]s-cluster-read
subjects:
  - kind: ServiceAccount
    name: %[2]s
    namespace: %[1]s
`, namespace, account)
}

func runClustersRBAC(prog string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(prog+" clusters rbac", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var namespace, account string
	var clusterRead bool
	fs.BoolVar(&clusterRead, "cluster-read", false, "also print a read-only ClusterRole and ClusterRoleBinding for nodes, namespaces and ingresses (cluster scope, needed to verify a deploy host)")
	fs.StringVar(&namespace, "namespace", "", "namespace the account may manage (required)")
	fs.StringVar(&account, "service-account", "levelrail", "ServiceAccount and Role name")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if !clusterRBACNameRe.MatchString(namespace) || !clusterRBACNameRe.MatchString(account) {
		_, _ = fmt.Fprintln(stderr, "--namespace and --service-account must be valid Kubernetes names")
		return exitUsage
	}
	_, _ = fmt.Fprintf(stdout, "# Deploy to namespace %[1]s: this role cannot create namespaces. Unless %[1]s is the default namespace\n# or carries the levelrail label, add it to k8s.allowed_namespaces on the server.\n", namespace)
	_, _ = fmt.Fprint(stdout, clustersRBACYAML(namespace, account))
	if clusterRead {
		_, _ = fmt.Fprint(stdout, clustersClusterReadYAML(namespace, account))
	}
	return exitOK
}
