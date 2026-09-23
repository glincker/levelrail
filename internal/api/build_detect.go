package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/build"
)

// detectFunc runs Railpack's own detection (no build, no image) against
// a git source, the seam handleDetectFramework calls through so tests
// can substitute a fake without a real network clone. Defaulted to
// build.Detect in NewRouter.
type detectFunc func(ctx context.Context, req build.DetectRequest) (*build.DetectResult, error)

// detectFrameworkRequest is POST /api/v1/build/detect's body: the same
// repo_url/ref shape gitBranchesRequest and triggerBuildRequest already
// use, so the wizard can call this with the exact values it already has
// once a repo and branch are picked, before any app exists to attach a
// build to.
type detectFrameworkRequest struct {
	RepoURL string `json:"repo_url"`
	Ref     string `json:"ref,omitempty"`
}

// detectFrameworkResponse is POST /api/v1/build/detect's success body.
// Detected is false whenever FrameworkName is empty, spelled out
// explicitly rather than left for the caller to infer from an empty
// string, since "" is also JSON's natural absent-value encoding.
type detectFrameworkResponse struct {
	Provider      string `json:"provider,omitempty"`
	FrameworkName string `json:"framework_name,omitempty"`
	Detected      bool   `json:"detected"`
}

// handleDetectFramework handles POST /api/v1/build/detect: a fast,
// build-free pre-flight check of what Railpack would detect for a git
// source, so the create-app-from-git wizard can show "Detected: Next.js"
// before the operator commits to a build type. Gated at AbilityDeploy,
// the same tier POST /api/v1/git/branches already requires for the same
// "not scoped to an existing app yet" reason (routes.go).
//
// A repo that can't be cloned, or against which Railpack detects
// nothing buildable, is not an error: it responds 200 with
// detected: false, so the wizard falls back to its existing manual
// build-type picker rather than surfacing a scary failure for what is
// often just an unsupported stack.
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
		rt.logger.Warn("api: detect framework failed", slog.String("error", err.Error()), slog.String("repo_url", req.RepoURL))
		writeJSON(w, http.StatusOK, detectFrameworkResponse{Detected: false})
		return
	}

	writeJSON(w, http.StatusOK, detectFrameworkResponse{
		Provider:      result.Provider,
		FrameworkName: result.FrameworkName,
		Detected:      result.FrameworkName != "",
	})
}
