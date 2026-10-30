package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

// v2FieldStore holds server-side apply ownership; *store.DB satisfies it.
type v2FieldStore interface {
	ListManagedFields(ctx context.Context, kind, name string) ([]store.ManagedField, error)
	ReplaceManagedFields(ctx context.Context, kind, name string, fields []store.ManagedField) error
	RecordResourceChange(ctx context.Context, c store.ResourceChange) (int64, error)
}

var (
	v2FieldManagerRe = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
	v2EnvPlaceRe     = regexp.MustCompile(`^\$\{\{ env\.([A-Za-z_][A-Za-z0-9_]*) \}\}$`)
)

// v2ApplyContentTypes are the bodies PATCH accepts: an apply patch is a partial manifest.
var v2ApplyContentTypes = map[string]bool{
	"application/apply-patch+yaml": true,
	"application/apply-patch+json": true,
	"application/yaml":             true,
	"application/x-yaml":           true,
	"application/json":             true,
}

type v2ManagedField struct {
	Manager string   `json:"manager"`
	Fields  []string `json:"fields"`
}

type v2ApplyResponse struct {
	v2Object
	ManagedFields []v2ManagedField `json:"managedFields,omitempty"`
	DryRun        bool             `json:"dryRun,omitempty"`
	Plan          *iac.Plan        `json:"plan,omitempty"`
	Result        *iac.ApplyResult `json:"result,omitempty"`
	Conflicts     []ssaConflict    `json:"conflicts,omitempty"`
	Issues        []iac.Issue      `json:"issues,omitempty"`
	Error         string           `json:"error,omitempty"`
}

type v2ApplyParams struct {
	manager string
	force   bool
	dryRun  bool
	version string
	spec    map[string]any
}

func groupManagedFields(fields []store.ManagedField) []v2ManagedField {
	by := map[string][]string{}
	for _, f := range fields {
		by[f.Manager] = append(by[f.Manager], f.Path)
	}
	out := make([]v2ManagedField, 0, len(by))
	for m, ps := range by {
		sort.Strings(ps)
		out = append(out, v2ManagedField{Manager: m, Fields: ps})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manager < out[j].Manager })
	return out
}

// handleApplyUnsupported answers PATCH on kinds without a write path yet.
func (s *v2Server) handleApplyUnsupported(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Allow", "GET")
	writeError(w, http.StatusMethodNotAllowed, fmt.Sprintf("server-side apply is not supported for %q yet: it works on apps and databases; stacks change through their stack apply", r.PathValue("kind")))
}

func parseV2ApplyRequest(w http.ResponseWriter, r *http.Request, kind, name string) (v2ApplyParams, bool) {
	var p v2ApplyParams
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if !v2ApplyContentTypes[ct] {
		writeError(w, http.StatusUnsupportedMediaType, "send a partial manifest as application/apply-patch+yaml or application/apply-patch+json")
		return p, false
	}
	q := r.URL.Query()
	p.manager = q.Get("fieldManager")
	if !v2FieldManagerRe.MatchString(p.manager) {
		writeError(w, http.StatusBadRequest, "fieldManager is required: up to 128 letters, digits, dots, colons, underscores or hyphens")
		return p, false
	}
	p.force = q.Get("force") == "true"
	switch dr := q.Get("dryRun"); dr {
	case "":
	case "All", "all", "true", "server":
		p.dryRun = true
	default:
		writeError(w, http.StatusBadRequest, "dryRun must be All")
		return p, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxIaCBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return p, false
	}
	var doc struct {
		Kind     string `yaml:"kind"`
		Metadata struct {
			Name            string `yaml:"name"`
			ResourceVersion string `yaml:"resourceVersion"`
		} `yaml:"metadata"`
		Spec map[string]any `yaml:"spec"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid manifest: "+err.Error())
		return p, false
	}
	if want := v2KindFor(kind); doc.Kind != "" && !strings.EqualFold(doc.Kind, want) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("kind %q does not match the %s route", doc.Kind, want))
		return p, false
	}
	if doc.Metadata.Name != "" && doc.Metadata.Name != name {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("metadata.name %q does not match %q in the path", doc.Metadata.Name, name))
		return p, false
	}
	if len(doc.Spec) == 0 {
		writeError(w, http.StatusBadRequest, "spec must hold at least one field")
		return p, false
	}
	p.spec = doc.Spec
	p.version = doc.Metadata.ResourceVersion
	if v := q.Get("resourceVersion"); v != "" {
		p.version = v
	}
	return p, true
}

// handleApply is server-side apply: merge only the manager's fields, refuse a
// change to a field another manager owns unless forced, then apply through IaC.
func (s *v2Server) handleApply(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fs, ok := s.rt.apps.(v2FieldStore)
		if s.changes == nil || !ok {
			writeError(w, http.StatusNotImplemented, "server-side apply is not available on this store")
			return
		}
		name := r.PathValue("name")
		p, ok := parseV2ApplyRequest(w, r, kind, name)
		if !ok {
			return
		}
		// One apply at a time keeps the version check and the write together.
		s.applyMu.Lock()
		defer s.applyMu.Unlock()
		s.applyLocked(w, r, fs, kind, name, p)
	}
}

func (s *v2Server) applyLocked(w http.ResponseWriter, r *http.Request, fs v2FieldStore, kind, name string, p v2ApplyParams) {
	ctx := r.Context()
	storeKind := v2StoreKind(kind)
	versions, err := s.changes.LatestResourceVersions(ctx, storeKind)
	if err != nil {
		s.rt.internalError(w, "api: v2 apply: versions", err)
		return
	}
	if p.version != "" && p.version != strconv.FormatInt(versions[name], 10) {
		writeError(w, http.StatusConflict, fmt.Sprintf("resourceVersion precondition failed: %s is at %d, not %s; re-read it and apply again", name, versions[name], p.version))
		return
	}
	docVersion, current, err := s.currentSpec(r, kind, name)
	if err != nil {
		var se *iac.StatusError
		if errors.As(err, &se) && se.Status >= 400 && se.Status < 500 {
			writeError(w, se.Status, se.Message)
			return
		}
		s.rt.internalError(w, "api: v2 apply: read "+kind, err)
		return
	}
	sent, err := flattenSpec(p.spec)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	owners, err := fs.ListManagedFields(ctx, storeKind, name)
	if err != nil {
		s.rt.internalError(w, "api: v2 apply: managed fields", err)
		return
	}
	out := ssaMerge{current: current, sent: sent, owners: owners, manager: p.manager, force: p.force}.run()
	if out.conflicts != nil && out.merged == nil {
		paths := make([]string, len(out.conflicts))
		for i, c := range out.conflicts {
			paths[i] = c.Path + " (" + c.Manager + ")"
		}
		writeJSON(w, http.StatusConflict, v2ApplyResponse{
			Error:     "apply conflicts with fields owned by other managers: " + strings.Join(paths, ", ") + "; pass force=true to take them over",
			Conflicts: out.conflicts,
		})
		return
	}
	s.applyMerged(w, r, fs, kind, name, docVersion, sent, out, p.dryRun)
}

func (s *v2Server) applyMerged(w http.ResponseWriter, r *http.Request, fs v2FieldStore, kind, name string, docVersion int, sent map[string]any, out ssaOutcome, dryRun bool) {
	ctx := r.Context()
	iacKind := iac.KindApp
	if kind == v2PathDatabases {
		iacKind = iac.KindDatabase
	}
	spec := unflattenSpec(out.merged)
	src, err := yaml.Marshal(map[string]any{"version": docVersion, "kind": string(iacKind), "metadata": map[string]any{"name": name}, "spec": spec})
	if err != nil {
		s.rt.internalError(w, "api: v2 apply: render", err)
		return
	}
	req := iacRequest{Files: []iacFile{{Name: name + ".yaml", Content: string(src)}}, Vars: s.keptEnvValues(ctx, kind, name, sent, out.merged)}
	res, opts, issues := req.build()
	if len(issues) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, v2ApplyResponse{Error: "the merged manifest is invalid", Issues: issues})
		return
	}
	st, err := iac.Load(ctx, s.rt.iacDoer(r), res, opts)
	if err != nil {
		s.rt.internalError(w, "api: v2 apply: plan", err)
		return
	}
	plan := iac.PlanFor(st, res, opts)
	if plan.Summary.Error > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, v2ApplyResponse{Error: "the plan has errors; nothing was applied", Plan: &plan})
		return
	}
	if ss := s.rt.stackStore(); ss != nil && !s.rt.refuseStackOwned(w, r, ss, res, plan) {
		return
	}
	managed := groupManagedFields(out.owners)
	resp := v2ApplyResponse{Plan: &plan}
	if dryRun {
		resp.DryRun = true
		resp.v2Object = v2Object{APIVersion: s.apiVersion(), Kind: v2KindFor(kind), Metadata: v2Metadata{Name: name, ManagedFields: managed}, Spec: spec}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	result, err := iac.Apply(ctx, s.rt.iacDoer(r), res, opts, plan.Hash)
	if errors.Is(err, iac.ErrPlanChanged) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		s.rt.internalError(w, "api: v2 apply", err)
		return
	}
	resp.Plan, resp.Result = nil, &result
	if !result.OK() {
		resp.Error = "some changes failed to apply; ownership was not recorded"
		writeJSON(w, http.StatusUnprocessableEntity, resp)
		return
	}
	if err := fs.ReplaceManagedFields(ctx, v2StoreKind(kind), name, out.owners); err != nil {
		s.rt.internalError(w, "api: v2 apply: record managed fields", err)
		return
	}
	s.noteApplied(ctx, fs, kind, name, spec, plan.Pending())
	items, err := s.listObjects(r, kind, name)
	if err != nil {
		s.rt.internalError(w, "api: v2 apply: re-read", err)
		return
	}
	if len(items) > 0 {
		resp.v2Object = items[0]
		resp.Metadata.ManagedFields = managed
	}
	writeJSON(w, http.StatusOK, resp)
}

// noteApplied emits the watch event for kinds whose write paths do not record
// changes themselves; app writes already do.
func (s *v2Server) noteApplied(ctx context.Context, fs v2FieldStore, kind, name string, spec map[string]any, changed bool) {
	if kind == v2PathApps || !changed {
		return
	}
	obj, err := json.Marshal(spec)
	if err != nil {
		return
	}
	if _, err := fs.RecordResourceChange(ctx, store.ResourceChange{Kind: v2StoreKind(kind), Name: name, Op: store.ResourceOpModified, Object: obj}); err != nil {
		s.rt.logger.Error("api: v2 apply: record change", "kind", kind, "name", name, "error", err.Error())
	}
}

// currentSpec reads the object as an IaC document, the shape apply writes back.
// A missing object is an empty spec, so apply can also create.
func (s *v2Server) currentSpec(r *http.Request, kind, name string) (int, map[string]any, error) {
	opts := iac.ExportOptions{IncludeEnvValues: s.rt.callerHasAbility(r, AbilityReadSensitive)}
	want := iac.KindDatabase
	if kind == v2PathApps {
		opts.App, want = name, iac.KindApp
	}
	exported, err := iac.Export(r.Context(), s.rt.iacDoer(r), opts)
	var se *iac.StatusError
	if errors.As(err, &se) && se.Status == http.StatusNotFound {
		return iac.SchemaVersion, map[string]any{}, nil
	}
	if err != nil {
		return 0, nil, err
	}
	for _, f := range exported.Files {
		if f.Kind != want {
			continue
		}
		var doc struct {
			Version  int                   `yaml:"version"`
			Metadata struct{ Name string } `yaml:"metadata"`
			Spec     map[string]any        `yaml:"spec"`
		}
		if err := yaml.Unmarshal([]byte(f.Content), &doc); err != nil {
			return 0, nil, fmt.Errorf("decode exported %s: %w", name, err)
		}
		if doc.Metadata.Name != name {
			continue
		}
		flat, err := flattenSpec(doc.Spec)
		return doc.Version, flat, err
	}
	return iac.SchemaVersion, map[string]any{}, nil
}

// keptEnvValues resolves the placeholders export wrote for env values the
// applier did not touch, so apply keeps them without ever showing them.
func (s *v2Server) keptEnvValues(ctx context.Context, kind, name string, sent, merged map[string]any) map[string]string {
	if kind != v2PathApps {
		return nil
	}
	var wanted []string
	for p, v := range merged {
		str, ok := v.(string)
		if !ok || !strings.Contains(str, "${{") {
			continue
		}
		m := v2EnvPlaceRe.FindStringSubmatch(str)
		// Only an env entry's own placeholder is restored; any other placeholder could copy a value somewhere readable.
		if m == nil || !strings.HasSuffix(p, "/env/"+ssaEscape(m[1])) {
			return nil
		}
		if _, mine := sent[p]; !mine {
			wanted = append(wanted, m[1])
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	svc, err := s.rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return nil
	}
	vars := map[string]string{}
	for _, k := range wanted {
		if v, ok := svc.Env[k]; ok {
			vars[k] = v
		}
	}
	return vars
}
