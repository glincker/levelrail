package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/frameworks"
)

type frameworkListResponse struct {
	Frameworks []frameworks.Preset `json:"frameworks"`
}

type frameworkDockerfileResponse struct {
	ID         string `json:"id"`
	Dockerfile string `json:"dockerfile"`
}

func (rt *Router) handleListFrameworks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, frameworkListResponse{Frameworks: frameworks.List()})
}

func (rt *Router) handleGetFramework(w http.ResponseWriter, r *http.Request) {
	p, ok := frameworks.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "framework preset not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (rt *Router) handleGetFrameworkDockerfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	text, ok := frameworks.DockerfileTemplate(id)
	if !ok {
		writeError(w, http.StatusNotFound, "no Dockerfile template for this framework preset")
		return
	}
	writeJSON(w, http.StatusOK, frameworkDockerfileResponse{ID: id, Dockerfile: text})
}
