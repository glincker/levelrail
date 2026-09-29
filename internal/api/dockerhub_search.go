package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dockerhub"
)

// dockerHubPageSize bounds both endpoints below to a single page: the
// picker (RegistryImagePicker.tsx) shows a short results list, not a
// paginated browse, the same "one page, no next-page UI" scope GET
// /api/v1/registry/repositories already has for the built-in registry.
const dockerHubPageSize = 25

// DockerHubClient is the surface GET /api/v1/dockerhub/search and GET
// /api/v1/dockerhub/repositories/{namespace}/{repo}/tags need: query
// Docker Hub's public, unauthenticated Hub API for repository search and
// tag listing. *dockerhub.Client satisfies this structurally.
type DockerHubClient interface {
	SearchRepositories(ctx context.Context, query string, pageSize int) ([]dockerhub.Repository, error)
	ListTags(ctx context.Context, namespace, repository string, pageSize int) ([]dockerhub.Tag, error)
}

type dockerHubSearchResponse struct {
	Results []dockerhub.Repository `json:"results"`
}

type dockerHubTagsResponse struct {
	Namespace  string          `json:"namespace"`
	Repository string          `json:"repository"`
	Tags       []dockerhub.Tag `json:"tags"`
}

// handleDockerHubSearch handles GET /api/v1/dockerhub/search?q=<query>:
// public Docker Hub repository search for the "docker image" app-creation
// step's picker (RegistryImagePicker.tsx), proxied server-side so the
// browser never calls hub.docker.com directly (CORS, and keeps any future
// API key or rate-limit handling out of the client). AbilityRead, same
// tier as GET /api/v1/registry/repositories: purely a read of a public,
// unauthenticated upstream, nothing sensitive ever crosses this boundary.
func (rt *Router) handleDockerHubSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeError(w, http.StatusBadRequest, "q query parameter is required")
		return
	}

	repos, err := rt.dockerHubClient.SearchRepositories(r.Context(), query, dockerHubPageSize)
	if err != nil {
		rt.logger.Error("api: docker hub search failed", slog.String("error", err.Error()), slog.String("query", query))
		writeError(w, http.StatusBadGateway, "could not reach Docker Hub")
		return
	}
	writeJSON(w, http.StatusOK, dockerHubSearchResponse{Results: repos})
}

// handleDockerHubTags handles GET
// /api/v1/dockerhub/repositories/{namespace}/{repo}/tags: every tag
// published for one Docker Hub repository, once the operator has picked
// one from the search results. namespace and repo are separate path
// segments here (unlike the built-in registry's own repository query
// parameter) because Docker Hub's own repo_name/tags URL shape already
// splits them the same way, and neither segment can itself contain "/".
func (rt *Router) handleDockerHubTags(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	repository := r.PathValue("repo")
	if namespace == "" || repository == "" {
		writeError(w, http.StatusBadRequest, "namespace and repo path segments are required")
		return
	}

	tags, err := rt.dockerHubClient.ListTags(r.Context(), namespace, repository, dockerHubPageSize)
	if errors.Is(err, dockerhub.ErrNotFound) {
		writeError(w, http.StatusNotFound, "repository not found on Docker Hub")
		return
	}
	if err != nil {
		rt.logger.Error("api: docker hub list tags failed", slog.String("error", err.Error()), slog.String("namespace", namespace), slog.String("repository", repository))
		writeError(w, http.StatusBadGateway, "could not reach Docker Hub")
		return
	}
	writeJSON(w, http.StatusOK, dockerHubTagsResponse{Namespace: namespace, Repository: repository, Tags: tags})
}
