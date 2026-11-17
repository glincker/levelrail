package k8sconn

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// SecretTypeDockerConfig is the Secret type the kubelet reads image pull credentials from.
const SecretTypeDockerConfig = //nolint:gosec // a Kubernetes type name, not a credential
"kubernetes.io/dockerconfigjson"

// ErrRegistryMismatch means the image is hosted on another registry than the pull secret.
var ErrRegistryMismatch = errors.New("k8sconn: the image registry does not match the pull secret registry")

var registryHostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?(:[0-9]{1,5})?$`)

const dockerHubHost = "docker.io"

// PullSecretName is the in-cluster Secret name for an app. One per app keeps removal exact.
func PullSecretName(app string) string { return app + "-registry" }

// NormalizeRegistryHost lowercases a registry and drops any scheme or path, and folds the
// Docker Hub aliases into one name. It returns "" when the result is not a host.
func NormalizeRegistryHost(s string) string {
	h := strings.ToLower(strings.TrimSpace(s))
	h = strings.TrimPrefix(strings.TrimPrefix(h, "https://"), "http://")
	h, _, _ = strings.Cut(h, "/")
	switch h {
	case "index.docker.io", "registry-1.docker.io", "registry.hub.docker.com":
		h = dockerHubHost
	}
	if !registryHostRe.MatchString(h) {
		return ""
	}
	return h
}

// ImageRegistryHost returns the registry an image reference is pulled from, using the same
// rule as the container runtime: a first segment with a dot, a port or localhost is a host.
func ImageRegistryHost(image string) string {
	first, _, found := strings.Cut(image, "/")
	if !found {
		return dockerHubHost
	}
	if strings.ContainsAny(first, ".:") || first == "localhost" {
		return NormalizeRegistryHost(first)
	}
	return dockerHubHost
}

// BuildPullSecret renders the dockerconfigjson Secret for an app. The credential lives only in
// the returned object and is never part of Rendered.YAML.
func BuildPullSecret(spec AppSpec, registry, username, password string) (map[string]any, error) {
	host := NormalizeRegistryHost(registry)
	if host == "" || username == "" || password == "" {
		return nil, fmt.Errorf("render: pull secret needs a registry, username and password")
	}
	if ImageRegistryHost(spec.Image) != host {
		return nil, ErrRegistryMismatch
	}
	key := host
	if host == dockerHubHost {
		key = "https://index.docker.io/v1/"
	}
	cfg, err := json.Marshal(map[string]any{"auths": map[string]any{key: map[string]any{
		"username": username, "password": password,
		"auth": base64.StdEncoding.EncodeToString([]byte(username + ":" + password)),
	}}})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"apiVersion": "v1", "kind": "Secret", "type": SecretTypeDockerConfig,
		"metadata": map[string]any{
			"name": spec.PullSecretName, "namespace": spec.Namespace, "labels": toAny(Labels(spec.Name, spec.InstanceID)),
		},
		"data": map[string]any{".dockerconfigjson": base64.StdEncoding.EncodeToString(cfg)},
	}, nil
}

// DeletePullSecret deletes a Secret only when it carries this app's labels, so a Secret of
// the same name that we did not create is never touched. A missing Secret is success.
func (c *Client) DeletePullSecret(ctx context.Context, namespace, name, app, instance string) error {
	path := "/api/v1/namespaces/" + url.PathEscape(namespace) + "/secrets/" + url.PathEscape(name)
	labels, found, err := c.getLabels(ctx, path)
	if err != nil || !found {
		return err
	}
	if instance == "" || !ownedBy(labels, app, instance) {
		return fmt.Errorf("secret %s: %w", name, ErrNotManaged)
	}
	_, err = c.Do(ctx, http.MethodDelete, path, nil)
	if isStatus(err, http.StatusNotFound) {
		return nil
	}
	return err
}
