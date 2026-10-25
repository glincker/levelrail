package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/stacktemplates"
)

// secretNameRe reads the names-only bullets of a bundle's SECRETS.md.
var secretNameRe = regexp.MustCompile("(?m)^- `([A-Za-z0-9_]+)`")

type stackTemplateVariable struct {
	Name        string `json:"name"`
	Default     string `json:"default"`
	Description string `json:"description"`
}

type stackTemplateSummary struct {
	ID            string                  `json:"id"`
	Title         string                  `json:"title"`
	Description   string                  `json:"description"`
	Variables     []stackTemplateVariable `json:"variables"`
	ResourceKinds []string                `json:"resource_kinds"`
	SecretNames   []string                `json:"secret_names"`
}

type stackTemplateDetail struct {
	stackTemplateSummary
	Files []iacFile `json:"files"`
}

type stackTemplateRenderRequest struct {
	Variables map[string]string `json:"variables"`
}

type stackTemplateRenderResponse struct {
	ID     string      `json:"id"`
	Files  []iacFile   `json:"files"`
	Issues []iac.Issue `json:"issues,omitempty"`
}

func stackTemplateVariables(t stacktemplates.Template) []stackTemplateVariable {
	vars, _ := t.Vars(nil)
	return []stackTemplateVariable{
		{Name: "name", Default: vars["name"], Description: "stack, project and app name"},
		{Name: "domain", Default: vars["domain"], Description: "host to serve"},
		{Name: "image", Default: vars["image"], Description: "prebuilt image to run"},
	}
}

func sortedStackFiles(files map[string][]byte) []iacFile {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]iacFile, len(names))
	for i, n := range names {
		out[i] = iacFile{Name: n, Content: string(files[n])}
	}
	return out
}

// summarizeStackTemplate renders with defaults; the bundles hold secret names only.
func summarizeStackTemplate(t stacktemplates.Template) (stackTemplateDetail, error) {
	files, err := t.Render(nil)
	if err != nil {
		return stackTemplateDetail{}, err
	}
	detail := stackTemplateDetail{
		stackTemplateSummary: stackTemplateSummary{
			ID: t.Name, Title: t.Name, Description: t.Description,
			Variables: stackTemplateVariables(t), ResourceKinds: []string{}, SecretNames: []string{},
		},
		Files: sortedStackFiles(files),
	}
	var sources []iac.Source
	for n, data := range files {
		if n == "SECRETS.md" {
			for _, m := range secretNameRe.FindAllSubmatch(data, -1) {
				detail.SecretNames = append(detail.SecretNames, string(m[1]))
			}
			continue
		}
		if strings.HasSuffix(n, ".yaml") {
			sources = append(sources, iac.Source{Name: n, Data: data})
		}
	}
	sort.Strings(detail.SecretNames)
	docs, _ := iac.ParseDocuments(sources)
	seen := map[string]bool{}
	for _, d := range docs {
		if k := string(d.Kind); k != "" && !seen[k] {
			seen[k] = true
			detail.ResourceKinds = append(detail.ResourceKinds, k)
		}
	}
	sort.Strings(detail.ResourceKinds)
	return detail, nil
}

func (rt *Router) handleListStackTemplates(w http.ResponseWriter, _ *http.Request) {
	list := stacktemplates.List()
	out := make([]stackTemplateSummary, 0, len(list))
	for _, t := range list {
		d, err := summarizeStackTemplate(t)
		if err != nil {
			rt.internalError(w, "api: stack template summary failed", err)
			return
		}
		out = append(out, d.stackTemplateSummary)
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": out})
}

func (rt *Router) handleGetStackTemplate(w http.ResponseWriter, r *http.Request) {
	t, ok := stacktemplates.Find(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "stack template not found")
		return
	}
	d, err := summarizeStackTemplate(t)
	if err != nil {
		rt.internalError(w, "api: stack template detail failed", err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// handleRenderStackTemplate renders and validates only; it never applies.
func (rt *Router) handleRenderStackTemplate(w http.ResponseWriter, r *http.Request) {
	t, ok := stacktemplates.Find(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "stack template not found")
		return
	}
	var req stackTemplateRenderRequest
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxIaCBodyBytes)).Decode(&req)
	if err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	files, err := t.Render(req.Variables)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp := stackTemplateRenderResponse{ID: t.Name, Files: sortedStackFiles(files)}
	var yamlFiles []iacFile
	for _, f := range resp.Files {
		// The Stack document configures the apply and is not a resource to build.
		if strings.HasSuffix(f.Name, ".yaml") && f.Name != "stack.yaml" {
			yamlFiles = append(yamlFiles, f)
		}
	}
	vars, _ := t.Vars(req.Variables)
	_, _, issues := iacRequest{Files: yamlFiles, Source: vars["name"]}.build()
	if len(issues) > 0 {
		resp.Issues = issues
		writeJSON(w, http.StatusUnprocessableEntity, resp)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
