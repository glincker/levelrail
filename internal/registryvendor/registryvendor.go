// Package registryvendor is the catalog of container registry vendors the
// credential flow offers presets for: default host, fixed username where
// the vendor defines one, and how long its tokens live.
package registryvendor

import "strings"

// Vendor describes one container registry vendor preset.
type Vendor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// DefaultHost is pre-filled in the credential form. Empty when the
	// host is per tenant and HostHint says what shape it takes.
	DefaultHost string `json:"default_host,omitempty"`
	HostHint    string `json:"host_hint,omitempty"`
	// FixedUsername is a username the vendor requires, such as AWS for ECR.
	FixedUsername string `json:"fixed_username,omitempty"`
	// TokenLifetimeHours is non-zero when the vendor's password expires
	// and must be re-issued, so the credential gets an expiry reminder.
	TokenLifetimeHours int `json:"token_lifetime_hours,omitempty"`
	// Brand is the web BrandIcon key for the vendor logo.
	Brand string `json:"brand"`
	// TokenHint is the minimum token scope to ask the operator for.
	TokenHint string `json:"token_hint,omitempty"`
}

// Generic is the ID of the any-registry preset.
const Generic = "generic"

var vendors = []Vendor{
	{ID: "dockerhub", Name: "Docker Hub", DefaultHost: "docker.io", Brand: "docker",
		TokenHint: "Personal access token with Read-only repository permission (Public Repo Read-only covers public images only), or an organization access token with scope-image-pull, scope-repository-read, scope-tag-read and scope-repository-list. Never a write scope."},
	{ID: "ghcr", Name: "GitHub Container Registry", DefaultHost: "ghcr.io", Brand: "github"},
	{ID: "gitlab", Name: "GitLab", DefaultHost: "registry.gitlab.com", HostHint: "registry.gitlab.com or your self-hosted registry host", Brand: "gitlab"},
	{ID: "ecr", Name: "AWS ECR", HostHint: "<account>.dkr.ecr.<region>.amazonaws.com", FixedUsername: "AWS", TokenLifetimeHours: 12, Brand: "ecr"},
	{ID: "gar", Name: "Google Artifact Registry", HostHint: "<region>-docker.pkg.dev", FixedUsername: "_json_key", Brand: "google-artifact-registry"},
	{ID: "acr", Name: "Azure Container Registry", HostHint: "<name>.azurecr.io", Brand: "azure-container-registry"},
	{ID: "harbor", Name: "Harbor", HostHint: "your Harbor host, for example harbor.example.com", Brand: "harbor"},
	{ID: "quay", Name: "Quay", DefaultHost: "quay.io", Brand: "quay"},
	{ID: "jfrog", Name: "JFrog Artifactory", HostHint: "<name>.jfrog.io", Brand: "jfrog"},
	{ID: "nexus", Name: "Sonatype Nexus", HostHint: "your Nexus Docker repository host and port"},
	{ID: Generic, Name: "Docker v2 registry", HostHint: "any registry that speaks the Docker Registry HTTP API v2", Brand: ""},
}

// Vendors returns the presets in display order.
func Vendors() []Vendor {
	return append([]Vendor(nil), vendors...)
}

// Find returns the preset with this ID.
func Find(id string) (Vendor, bool) {
	for _, v := range vendors {
		if v.ID == id {
			return v, true
		}
	}
	return Vendor{}, false
}

// IsDockerHubHost reports whether a registry host string names Docker Hub,
// with or without a scheme, port or path.
func IsDockerHubHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimPrefix(strings.TrimPrefix(h, "https://"), "http://")
	h, _, _ = strings.Cut(h, "/")
	h, _, _ = strings.Cut(h, ":")
	switch h {
	case "docker.io", "index.docker.io", "registry-1.docker.io", "registry.hub.docker.com", "hub.docker.com":
		return true
	}
	return false
}

// DockerHubRejected is the health text stored when Docker Hub refuses a token.
const DockerHubRejected = "token rejected by Docker Hub"
