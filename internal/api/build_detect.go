package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/build"
)

// Seam so tests can swap in a fake for the real clone-and-detect call.
type detectFunc func(ctx context.Context, req build.DetectRequest) (*build.DetectResult, error)

// Same repo_url and ref shape as the branches and build requests, so the wizard reuses its values.
type detectFrameworkRequest struct {
	RepoURL string `json:"repo_url"`
	Ref     string `json:"ref,omitempty"`
}

// Detected is explicit, since an empty FrameworkName is also JSON's absent-value encoding.
type detectFrameworkResponse struct {
	Provider      string `json:"provider,omitempty"`
	FrameworkName string `json:"framework_name,omitempty"`
	Detected      bool   `json:"detected"`
}

// An unclonable or undetectable repo returns 200 with detected: false, so the wizard falls back to manual picking.
func (rt *Router) handleDetectFramework(w http.ResponseWriter, r *http.Request) {
	var req detectFrameworkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RepoURL == "" {
		writeError(w, http.StatusBadRequest, "repo_url is required")
		return
	}

	result, err := rt.detect(r.Context(), build.DetectRequest{RepoURL: req.RepoURL, Ref: req.Ref})
	if err != nil {
		rt.logger.Warn("api: detect framework failed", slog.String("error", err.Error()), slog.String("repo_url", redactURLCredentials(req.RepoURL)))
		writeJSON(w, http.StatusOK, detectFrameworkResponse{Detected: false})
		return
	}

	writeJSON(w, http.StatusOK, detectFrameworkResponse{
		Provider:      result.Provider,
		FrameworkName: result.FrameworkName,
		Detected:      result.FrameworkName != "",
	})
}
