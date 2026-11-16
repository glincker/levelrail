// Package k8sconn is a small Kubernetes API client built on net/http. It
// avoids client-go and kubectl so no exec plugin or control plane file is
// ever read on behalf of an uploaded kubeconfig.
package k8sconn
