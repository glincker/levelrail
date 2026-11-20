package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dataexport"
	"github.com/GLINCKER/levelrail/internal/observability"
	"github.com/GLINCKER/levelrail/internal/store"
)

// presetSignalView is what a form needs to know about a signal.
type presetSignalView struct {
	Signal  string `json:"signal"`
	Default bool   `json:"default"`
}

type presetView struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	DocsAnchor  string                `json:"docs_anchor"`
	Fields      []observability.Field `json:"fields"`
	Signals     []presetSignalView    `json:"signals"`
}

type presetCatalogResource struct {
	Presets []presetView `json:"presets"`
}

func (rt *Router) handleListObservabilityPresets(w http.ResponseWriter, _ *http.Request) {
	cat := observability.Catalog()
	out := presetCatalogResource{Presets: make([]presetView, 0, len(cat))}
	for _, p := range cat {
		v := presetView{ID: p.ID, Name: p.Name, Description: p.Description, DocsAnchor: p.DocsAnchor, Fields: p.Fields}
		for _, s := range p.Signals {
			v.Signals = append(v.Signals, presetSignalView{Signal: s.Signal, Default: s.Default})
		}
		out.Presets = append(out.Presets, v)
	}
	writeJSON(w, http.StatusOK, out)
}

type obsSignalResource struct {
	Signal        string   `json:"signal"`
	DestinationID string   `json:"destination_id"`
	Kind          string   `json:"kind"`
	Endpoint      string   `json:"endpoint"`
	Enabled       bool     `json:"enabled"`
	HeaderNames   []string `json:"header_names"`
	HasCredential bool     `json:"has_credential"`
	LastSuccessAt string   `json:"last_success_at"`
	LastError     string   `json:"last_error"`
	SentTotal     int64    `json:"sent_total"`
	CircuitOpen   bool     `json:"circuit_open"`
}

// observabilityExportResource never carries a credential value.
type observabilityExportResource struct {
	ID         string              `json:"id"`
	Preset     string              `json:"preset"`
	PresetName string              `json:"preset_name"`
	Signals    []obsSignalResource `json:"signals"`
}

type createObservabilityExportRequest struct {
	Preset  string            `json:"preset"`
	Name    string            `json:"name"`
	Fields  map[string]string `json:"fields"`
	Signals []string          `json:"signals"`
}

func (rt *Router) writeObservabilityError(w http.ResponseWriter, op string, err error) {
	if errors.Is(err, observability.ErrInvalid) {
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), observability.ErrInvalid.Error()+": "))
		return
	}
	rt.writeExportError(w, op, err)
}

// observabilityGroups regroups the stored destinations made from presets.
func (rt *Router) observabilityGroups(r *http.Request, svc DataExportService) ([]observabilityExportResource, map[string][]store.ExportDestination, error) {
	dests, err := svc.List(r.Context())
	if err != nil {
		return nil, nil, err
	}
	raw := map[string][]store.ExportDestination{}
	res := map[string]*observabilityExportResource{}
	for _, d := range dests {
		name, preset, signal, ok := observability.ParseDestinationName(d.Name)
		if !ok {
			continue
		}
		g, ok := res[name]
		if !ok {
			pn := preset
			if p, found := observability.Lookup(preset); found {
				pn = p.Name
			}
			g = &observabilityExportResource{ID: name, Preset: preset, PresetName: pn, Signals: []obsSignalResource{}}
			res[name] = g
		}
		st, serr := svc.Status(r.Context(), d.ID)
		if serr != nil {
			st = store.ExportStatus{DestinationID: d.ID}
		}
		names := d.Options.HeaderNames
		if names == nil {
			names = []string{}
		}
		g.Signals = append(g.Signals, obsSignalResource{
			Signal: signal, DestinationID: d.ID, Kind: string(d.Kind), Endpoint: d.Endpoint, Enabled: d.Enabled,
			HeaderNames: names, HasCredential: len(names) > 0, LastSuccessAt: st.LastSuccessAt, LastError: st.LastError,
			SentTotal: st.SentTotal, CircuitOpen: st.CircuitOpen,
		})
		raw[name] = append(raw[name], d)
	}
	out := make([]observabilityExportResource, 0, len(res))
	for _, g := range res {
		sort.Slice(g.Signals, func(i, j int) bool { return g.Signals[i].Signal < g.Signals[j].Signal })
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, raw, nil
}

func (rt *Router) handleListObservabilityExports(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.dataExportOrNotImplemented(w)
	if !ok {
		return
	}
	out, _, err := rt.observabilityGroups(r, svc)
	if err != nil {
		rt.writeObservabilityError(w, "list observability exports", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleCreateObservabilityExport(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.dataExportOrNotImplemented(w)
	if !ok {
		return
	}
	var req createObservabilityExportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	preset, found := observability.Lookup(req.Preset)
	if !found {
		writeError(w, http.StatusBadRequest, "unknown preset")
		return
	}
	if !observability.ValidName(req.Name) {
		writeError(w, http.StatusBadRequest, "name must be 1 to 32 lowercase letters, digits or dashes")
		return
	}
	rendered, err := preset.Render(req.Name, req.Fields, req.Signals)
	if err != nil {
		rt.writeObservabilityError(w, "render observability export", err)
		return
	}
	_, existing, err := rt.observabilityGroups(r, svc)
	if err != nil {
		rt.writeObservabilityError(w, "create observability export", err)
		return
	}
	if len(existing[req.Name]) > 0 {
		writeError(w, http.StatusConflict, "an observability export with this name already exists")
		return
	}
	var created []string
	for _, rd := range rendered {
		d, err := svc.Create(r.Context(), rd.Input)
		if err != nil {
			for _, id := range created {
				_ = svc.Delete(r.Context(), id)
			}
			rt.writeObservabilityError(w, "create observability export", err)
			return
		}
		created = append(created, d.ID)
	}
	groups, _, err := rt.observabilityGroups(r, svc)
	if err != nil {
		rt.writeObservabilityError(w, "create observability export", err)
		return
	}
	for _, g := range groups {
		if g.ID == req.Name {
			writeJSON(w, http.StatusCreated, g)
			return
		}
	}
	writeError(w, http.StatusInternalServerError, "created export not found")
}

func (rt *Router) handleDeleteObservabilityExport(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.dataExportOrNotImplemented(w)
	if !ok {
		return
	}
	_, groups, err := rt.observabilityGroups(r, svc)
	if err != nil {
		rt.writeObservabilityError(w, "delete observability export", err)
		return
	}
	dests := groups[r.PathValue("id")]
	if len(dests) == 0 {
		writeError(w, http.StatusNotFound, "observability export not found")
		return
	}
	for _, d := range dests {
		if err := svc.Delete(r.Context(), d.ID); err != nil && !errors.Is(err, store.ErrExportDestinationNotFound) {
			rt.writeObservabilityError(w, "delete observability export", err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

type obsTestResult struct {
	Signal string `json:"signal"`
	OK     bool   `json:"ok"`
	Status int    `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
	Hint   string `json:"hint,omitempty"`
	Fix    string `json:"fix,omitempty"`
}

type obsTestResponse struct {
	ID      string          `json:"id"`
	OK      bool            `json:"ok"`
	Results []obsTestResult `json:"results"`
}

// handleTestObservabilityExport probes every signal of one export. A delivery
// failure is the answer, so it returns 200 with ok=false.
func (rt *Router) handleTestObservabilityExport(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.dataExportOrNotImplemented(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	_, groups, err := rt.observabilityGroups(r, svc)
	if err != nil {
		rt.writeObservabilityError(w, "test observability export", err)
		return
	}
	dests := groups[id]
	if len(dests) == 0 {
		writeError(w, http.StatusNotFound, "observability export not found")
		return
	}
	resp := obsTestResponse{ID: id, OK: true, Results: []obsTestResult{}}
	for _, d := range dests {
		_, presetID, signal, _ := observability.ParseDestinationName(d.Name)
		preset, _ := observability.Lookup(presetID)
		res := obsTestResult{Signal: signal, OK: true}
		if terr := svc.Test(r.Context(), d.ID); terr != nil {
			msg := terr.Error()
			if len(msg) > 300 {
				msg = msg[:300]
			}
			res.OK, res.Error = false, msg
			res.Status, res.Hint = preset.Hint(terr)
			res.Fix = string(dataexport.ClassifyTestError(terr))
			resp.OK = false
		}
		resp.Results = append(resp.Results, res)
	}
	sort.Slice(resp.Results, func(i, j int) bool { return resp.Results[i].Signal < resp.Results[j].Signal })
	writeJSON(w, http.StatusOK, resp)
}
