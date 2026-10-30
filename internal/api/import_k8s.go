package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/k8simport"
)

const k8sImportFile = "k8s-import.yaml"

type k8sImportRequest struct {
	Manifest string `json:"manifest"`
}

type k8sImportResponse struct {
	DryRun    bool                   `json:"dry_run"`
	Applied   bool                   `json:"applied"`
	Blockers  int                    `json:"blockers"`
	Documents string                 `json:"documents"`
	Apps      []k8simport.AppSummary `json:"apps"`
	Findings  []k8simport.Finding    `json:"findings"`
	Result    *iac.ApplyResult       `json:"result,omitempty"`
	Issues    []iac.Issue            `json:"issues,omitempty"`
}

// readK8sManifest accepts raw YAML or {"manifest": "..."} and caps the size
// before any parsing.
func readK8sManifest(w http.ResponseWriter, r *http.Request, maxBytes int) ([]byte, bool) {
	// JSON string escaping can inflate the body, so the wire cap is a multiple of the manifest cap.
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(maxBytes)*2+1024))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "manifest is too large")
		return nil, false
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var req k8sImportRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return nil, false
		}
		raw = []byte(req.Manifest)
	}
	return raw, true
}

// handleImportK8s converts Kubernetes YAML and, unless dry_run, applies it
// through the same loopback path as the apply route, so the caller's own
// abilities gate every resource it creates.
func (rt *Router) handleImportK8s(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dryRun := q.Get("dry_run") == "true" || q.Get("dry_run") == "1"
	force := q.Get("force") == "true" || q.Get("force") == "1"
	opts := k8simport.Options{AppPrefix: q.Get("app_prefix"), NamespaceAs: q.Get("namespace_as"), Project: q.Get("project")}
	raw, ok := readK8sManifest(w, r, k8simport.LimitsFromEnv().MaxBytes)
	if !ok {
		return
	}
	res, err := k8simport.Convert(raw, opts)
	if errors.Is(err, k8simport.ErrInput) {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: k8s import failed", err)
		return
	}
	out := k8sImportResponse{DryRun: dryRun, Blockers: res.Blockers(), Documents: res.Documents, Apps: res.Apps, Findings: res.Findings}
	if dryRun {
		writeJSON(w, http.StatusOK, out)
		return
	}
	if out.Blockers > 0 && !force {
		writeJSON(w, http.StatusUnprocessableEntity, out)
		return
	}
	if len(res.Apps) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, out)
		return
	}
	req := iacRequest{Files: []iacFile{{Name: k8sImportFile, Content: res.Documents}}, Secrets: res.Secrets()}
	built, iacOpts, issues := req.build()
	if len(issues) > 0 {
		out.Issues = issues
		writeJSON(w, http.StatusUnprocessableEntity, out)
		return
	}
	applied, err := iac.Apply(r.Context(), rt.iacDoer(r), built, iacOpts, "")
	if err != nil {
		rt.internalError(w, "api: k8s import apply failed", err)
		return
	}
	out.Applied, out.Result = true, &applied
	writeJSON(w, http.StatusOK, out)
}
