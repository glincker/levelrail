package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"sync"

	"github.com/GLINCKER/levelrail/internal/iac"
)

const (
	maxIaCBodyBytes = 8 << 20
	maxIaCFiles     = 200
)

var iacSourceRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,48}$`)

// iacDeps holds the in-process handler apply talks to, built on first use.
type iacDeps struct {
	once sync.Once
	h    http.Handler
}

type iacFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type iacRequest struct {
	Files            []iacFile         `json:"files"`
	Source           string            `json:"source,omitempty"`
	Project          string            `json:"project,omitempty"`
	Prune            bool              `json:"prune,omitempty"`
	Vars             map[string]string `json:"vars,omitempty"`
	Secrets          map[string]string `json:"secrets,omitempty"`
	NoDeploy         bool              `json:"no_deploy,omitempty"`
	ContinueOnError  bool              `json:"continue_on_error,omitempty"`
	ExpectedPlanHash string            `json:"expected_plan_hash,omitempty"`
}

type iacPlanResponse struct {
	Plan   *iac.Plan   `json:"plan,omitempty"`
	Issues []iac.Issue `json:"issues,omitempty"`
}

type iacApplyResponse struct {
	Result *iac.ApplyResult `json:"result,omitempty"`
	Issues []iac.Issue      `json:"issues,omitempty"`
}

func (rt *Router) handleIaCSchema(w http.ResponseWriter, _ *http.Request) {
	raw, err := iac.SchemaJSON()
	if err != nil {
		rt.internalError(w, "api: iac schema failed", err)
		return
	}
	w.Header().Set("Content-Type", "application/schema+json")
	_, _ = w.Write(raw)
}

func (rt *Router) iacDoer(r *http.Request) iac.Doer {
	rt.iac.once.Do(func() { rt.iac.h = rt.Handler() })
	return &loopbackDoer{h: rt.iac.h, orig: r}
}

func decodeIaCRequest(w http.ResponseWriter, r *http.Request) (iacRequest, bool) {
	var req iacRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxIaCBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return req, false
	}
	if len(req.Files) == 0 || len(req.Files) > maxIaCFiles {
		writeError(w, http.StatusBadRequest, "files must hold between 1 and "+strconv.Itoa(maxIaCFiles)+" entries")
		return req, false
	}
	if req.Source != "" && !iacSourceRe.MatchString(req.Source) {
		writeError(w, http.StatusBadRequest, "source may hold letters, digits, dots, underscores and hyphens, up to 48 characters")
		return req, false
	}
	if req.Prune && req.Source == "" {
		writeError(w, http.StatusBadRequest, "prune needs a source: only resources this source created are ever deleted")
		return req, false
	}
	return req, true
}

func (req iacRequest) build() ([]*iac.Resource, iac.Options, []iac.Issue) {
	sources := make([]iac.Source, len(req.Files))
	for i, f := range req.Files {
		sources[i] = iac.Source{Name: f.Name, Data: []byte(f.Content)}
	}
	opts := iac.Options{Vars: req.Vars, Source: req.Source, Project: req.Project, Secrets: req.Secrets, Prune: req.Prune, NoDeploy: req.NoDeploy, ContinueOnError: req.ContinueOnError}
	docs, issues := iac.ParseDocuments(sources)
	if len(issues) > 0 {
		return nil, opts, issues
	}
	if req.Project != "" {
		docs = iac.FilterProject(docs, req.Project)
	}
	res, issues := iac.Build(docs, opts)
	return res, opts, issues
}

func (rt *Router) handleIaCPlan(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeIaCRequest(w, r)
	if !ok {
		return
	}
	res, opts, issues := req.build()
	if len(issues) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, iacPlanResponse{Issues: issues})
		return
	}
	st, err := iac.Load(r.Context(), rt.iacDoer(r), res, opts)
	if err != nil {
		rt.internalError(w, "api: iac plan failed", err)
		return
	}
	plan := iac.PlanFor(st, res, opts)
	writeJSON(w, http.StatusOK, iacPlanResponse{Plan: &plan})
}

func (rt *Router) handleIaCApply(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeIaCRequest(w, r)
	if !ok {
		return
	}
	res, opts, issues := req.build()
	if len(issues) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, iacApplyResponse{Issues: issues})
		return
	}
	out, err := iac.Apply(r.Context(), rt.iacDoer(r), res, opts, req.ExpectedPlanHash)
	if errors.Is(err, iac.ErrPlanChanged) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: iac apply failed", err)
		return
	}
	writeJSON(w, http.StatusOK, iacApplyResponse{Result: &out})
}

func (rt *Router) handleIaCExport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	opts := iac.ExportOptions{Project: q.Get("project"), App: q.Get("app"), IncludeEnvValues: q.Get("include_env_values") != "false"}
	out, err := iac.Export(r.Context(), rt.iacDoer(r), opts)
	if err != nil {
		var se *iac.StatusError
		if errors.As(err, &se) && se.Status >= 400 && se.Status < 500 {
			writeError(w, se.Status, se.Message)
			return
		}
		rt.internalError(w, "api: iac export failed", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// loopbackDoer runs REST calls through the full handler chain with the
// caller's own credentials, so every ability and per-resource policy check
// applies exactly as if the caller had made the request themselves.
type loopbackDoer struct {
	h    http.Handler
	orig *http.Request
}

type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header         { return b.header }
func (b *bufferedResponse) WriteHeader(status int)      { b.status = status }
func (b *bufferedResponse) Write(p []byte) (int, error) { return b.body.Write(p) }

func (l *loopbackDoer) Do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, path, rdr)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	for _, h := range []string{"Authorization", "Cookie", "User-Agent", "X-Forwarded-For", "X-Real-Ip"} {
		if v := l.orig.Header.Values(h); len(v) > 0 {
			req.Header[h] = v
		}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.RemoteAddr, req.Host, req.TLS = l.orig.RemoteAddr, l.orig.Host, l.orig.TLS
	rec := &bufferedResponse{header: http.Header{}, status: http.StatusOK}
	l.h.ServeHTTP(rec, req)
	if rec.status < 200 || rec.status >= 300 {
		return &iac.StatusError{Status: rec.status, Message: errorMessage(rec.body.Bytes(), rec.status)}
	}
	if out != nil && rec.body.Len() > 0 {
		if err := json.Unmarshal(rec.body.Bytes(), out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func errorMessage(raw []byte, status int) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error != "" {
		return e.Error
	}
	return http.StatusText(status)
}
