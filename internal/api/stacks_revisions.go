package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/iac"
)

const rollbackWarning = "secret values and database contents are not rolled back; only the declared configuration is re-applied"

type stackRevisionDetail struct {
	stackRevisionSummary
	Files  []iacFile       `json:"files,omitempty"`
	Plan   json.RawMessage `json:"plan,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

type stackDiffChange struct {
	Kind   iac.Kind          `json:"kind"`
	Name   string            `json:"name"`
	Scope  string            `json:"scope,omitempty"`
	Action string            `json:"action"`
	Fields []iac.FieldChange `json:"fields,omitempty"`
}

type stackDiffResponse struct {
	Stack    string            `json:"stack"`
	Revision int               `json:"revision"`
	Against  string            `json:"against"`
	Changes  []stackDiffChange `json:"changes"`
	PlanHash string            `json:"plan_hash,omitempty"`
}

func revisionNumber(w http.ResponseWriter, r *http.Request) (int, bool) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 1 {
		writeError(w, http.StatusBadRequest, "revision number must be a positive integer")
		return 0, false
	}
	return n, true
}

func (rt *Router) handleStackRevisions(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	s, ok := rt.loadStack(w, r, ss)
	if !ok {
		return
	}
	limit := stackHistoryLimit()
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n < limit {
		limit = n
	}
	revs, err := ss.ListStackRevisions(r.Context(), s.ID, limit)
	if err != nil {
		rt.writeStackError(w, "history", err)
		return
	}
	out := make([]stackRevisionSummary, 0, len(revs))
	for _, v := range revs {
		out = append(out, summarizeRevision(v))
	}
	writeJSON(w, http.StatusOK, map[string]any{"stack": s.Name, "pinned_revision": s.PinnedRevision, "revisions": out})
}

func decodeRenderedFiles(raw []byte) ([]iacFile, error) {
	var files []iacFile
	if err := json.Unmarshal(raw, &files); err != nil {
		return nil, fmt.Errorf("decode rendered documents: %w", err)
	}
	return files, nil
}

func (rt *Router) handleStackRevision(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	s, ok := rt.loadStack(w, r, ss)
	if !ok {
		return
	}
	n, ok := revisionNumber(w, r)
	if !ok {
		return
	}
	v, err := ss.GetStackRevision(r.Context(), s.ID, n)
	if err != nil {
		rt.writeStackError(w, "revision", err)
		return
	}
	files, err := decodeRenderedFiles(v.RenderedDocs)
	if err != nil {
		rt.internalError(w, "api: stack revision documents unreadable", err)
		return
	}
	writeJSON(w, http.StatusOK, stackRevisionDetail{stackRevisionSummary: summarizeRevision(v), Files: files, Plan: v.PlanJSON, Result: v.ResultJSON})
}

// stackResources parses stored documents into resources keyed by identity.
func stackResources(files []iacFile, stack, project string) (map[string]*iac.Resource, error) {
	res, _, issues := iacRequest{Files: files, Source: stack, Project: project}.build()
	if len(issues) > 0 {
		return nil, fmt.Errorf("stored documents no longer validate: %s", issues[0].Message)
	}
	out := make(map[string]*iac.Resource, len(res))
	for _, d := range res {
		out[d.Key()] = d
	}
	return out, nil
}

// opaqueDiffKeys never show values, matching the plan's own env handling.
var opaqueDiffKeys = map[string]bool{"env": true, "labels": true, "command": true}

func diffResources(from, to map[string]*iac.Resource) []stackDiffChange {
	keys := map[string]bool{}
	for k := range from {
		keys[k] = true
	}
	for k := range to {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	var out []stackDiffChange
	for _, k := range sorted {
		a, b := from[k], to[k]
		switch {
		case a == nil:
			out = append(out, stackDiffChange{Kind: b.Kind, Name: b.Name, Scope: b.Scope, Action: "added"})
		case b == nil:
			out = append(out, stackDiffChange{Kind: a.Kind, Name: a.Name, Scope: a.Scope, Action: "removed"})
		default:
			if fields := diffFieldMaps(a.Fields, b.Fields); len(fields) > 0 {
				out = append(out, stackDiffChange{Kind: b.Kind, Name: b.Name, Scope: b.Scope, Action: "changed", Fields: fields})
			}
		}
	}
	return out
}

func diffFieldMaps(a, b map[string]any) []iac.FieldChange {
	enc := func(v any) string { raw, _ := json.Marshal(v); return string(raw) }
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	var out []iac.FieldChange
	for _, k := range sorted {
		av, aok := a[k]
		bv, bok := b[k]
		if aok && bok && enc(av) == enc(bv) {
			continue
		}
		fc := iac.FieldChange{Path: k, Op: iac.OpChange}
		switch {
		case !aok:
			fc.Op = iac.OpAdd
		case !bok:
			fc.Op = iac.OpRemove
		}
		if !opaqueDiffKeys[k] {
			if aok {
				fc.Old = enc(av)
			}
			if bok {
				fc.New = enc(bv)
			}
		}
		out = append(out, fc)
	}
	return out
}

func (rt *Router) handleStackDiff(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	s, ok := rt.loadStack(w, r, ss)
	if !ok {
		return
	}
	n, ok := revisionNumber(w, r)
	if !ok {
		return
	}
	v, err := ss.GetStackRevision(r.Context(), s.ID, n)
	if err != nil {
		rt.writeStackError(w, "diff", err)
		return
	}
	files, err := decodeRenderedFiles(v.RenderedDocs)
	if err != nil {
		rt.internalError(w, "api: stack revision documents unreadable", err)
		return
	}
	project := rt.projectName(r.Context(), s.ProjectID)
	against := r.URL.Query().Get("against")
	resp := stackDiffResponse{Stack: s.Name, Revision: n, Against: "live", Changes: []stackDiffChange{}}
	if against == "" || against == "live" {
		res, opts, issues := iacRequest{Files: files, Source: s.Name, Project: project}.build()
		if len(issues) > 0 {
			writeJSON(w, http.StatusUnprocessableEntity, iacPlanResponse{Issues: issues})
			return
		}
		st, err := iac.Load(r.Context(), rt.iacDoer(r), res, opts)
		if err != nil {
			rt.internalError(w, "api: stack diff failed", err)
			return
		}
		plan := iac.PlanFor(st, res, opts)
		resp.PlanHash = plan.Hash
		for _, c := range plan.Changes {
			if c.Action == iac.ActionNoop {
				continue
			}
			resp.Changes = append(resp.Changes, stackDiffChange{Kind: c.Kind, Name: c.Name, Scope: c.Scope, Action: string(c.Action), Fields: c.Fields})
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	m, err := strconv.Atoi(against)
	if err != nil || m < 1 {
		writeError(w, http.StatusBadRequest, "against must be a revision number or \"live\"")
		return
	}
	other, err := ss.GetStackRevision(r.Context(), s.ID, m)
	if err != nil {
		rt.writeStackError(w, "diff", err)
		return
	}
	otherFiles, err := decodeRenderedFiles(other.RenderedDocs)
	if err != nil {
		rt.internalError(w, "api: stack revision documents unreadable", err)
		return
	}
	from, err := stackResources(otherFiles, s.Name, project)
	if err == nil {
		var to map[string]*iac.Resource
		if to, err = stackResources(files, s.Name, project); err == nil {
			resp.Against = against
			resp.Changes = append(resp.Changes, diffResources(from, to)...)
			writeJSON(w, http.StatusOK, resp)
			return
		}
	}
	writeError(w, http.StatusUnprocessableEntity, err.Error())
}

func (rt *Router) handleStackRollback(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	s, ok := rt.loadStack(w, r, ss)
	if !ok {
		return
	}
	n, ok := revisionNumber(w, r)
	if !ok {
		return
	}
	req, ok := decodeStackRequest(w, r, true)
	if !ok {
		return
	}
	target, err := ss.GetStackRevision(r.Context(), s.ID, n)
	if err != nil {
		rt.writeStackError(w, "rollback", err)
		return
	}
	files, err := decodeRenderedFiles(target.RenderedDocs)
	if err != nil {
		rt.internalError(w, "api: stack revision documents unreadable", err)
		return
	}
	// A rollback re-applies exactly what revision n declared: no new vars, no prune.
	req.Files, req.Vars, req.Prune = files, nil, false
	rt.runStack(w, r, ss, stackRun{stack: s, req: req, rollbackOf: n, action: auditStackRollback, warnings: []string{rollbackWarning}})
}
