package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbaccess"
	"github.com/GLINCKER/levelrail/internal/exposure"
	"github.com/GLINCKER/levelrail/internal/store"
)

type databaseRuleInput struct {
	Source      string `json:"source"`
	Description string `json:"description"`
}

type databaseRulesRequest struct {
	Allow           []databaseRuleInput `json:"allow"`
	LocalContainers bool                `json:"local_containers"`
	Confirm         bool                `json:"confirm"`
}

type databaseRulesPlanResponse struct {
	exposurePlanResource
	Descriptions map[string]string `json:"descriptions,omitempty"`
}

const (
	errNoPublishedPort = "this database has no published port, so there is nothing to restrict; publish one first or leave it private"
	maxRuleDescription = 120
	maxRuleSources     = 64
)

// databaseRules reads the stored restriction for the database's published
// port and compares it with what the firewall actually holds.
func (rt *Router) databaseRules(ctx context.Context, _ *http.Request, d *store.DesiredDatabase, f *exposureFindingResource, chain exposure.Chain) databaseRulesResource {
	res := databaseRulesResource{Protocol: protoTCP, Allow: []databaseRuleResource{}}
	if !d.PubliclyAccessible || d.PublicPort == 0 {
		res.CannotReason = errNoPublishedPort
		return res
	}
	res.Port = d.PublicPort
	switch {
	case rt.exposure == nil || rt.exposureStore == nil:
		res.CannotReason = "the exposure controls are not configured on this control plane"
		return res
	case !rt.isLocalNode(d.NodeID):
		res.CannotReason = "Rules can only be applied on the control plane's own host in this version."
		return res
	}
	notes, err := rt.dbAccess.ListDatabaseNetworkRuleNotes(ctx, d.Name)
	if err != nil {
		rt.logger.Warn("api: database network: rule notes failed", slog.String("name", d.Name), slog.String("error", err.Error()))
	}
	if r, ok := rt.exposureRestrictionMap(ctx)[restrictionKey(d.PublicPort, protoTCP)]; ok {
		res.Active = true
		for _, src := range r.Allow {
			res.Allow = append(res.Allow, databaseRuleResource{Source: src, Description: notes[src]})
		}
		if f != nil {
			res.Missing, res.Extra = diffSources(r.Allow, f.AllowedSources, f.Class == exposure.ClassRestricted)
		}
	}
	if f == nil {
		res.CannotReason = "the firewall audit has no finding for this port yet"
		return res
	}
	res.CanRestrict, res.CannotReason = rt.exposureCanRestrict(f.Finding, exposureNodeRef{name: localNodeLabel, local: true}, chain)
	if !res.CanRestrict && res.Active && f.Class == exposure.ClassRestricted {
		res.CannotReason = ""
	}
	return res
}

func diffSources(want, have []string, restricted bool) (missing, extra []string) {
	if !restricted {
		return slices.Clone(want), nil
	}
	for _, w := range want {
		if !slices.Contains(have, w) {
			missing = append(missing, w)
		}
	}
	for _, h := range have {
		if !slices.Contains(want, h) {
			extra = append(extra, h)
		}
	}
	return missing, extra
}

// databaseRuleRestriction validates the request and turns it into the
// exposure manager's input, reusing its normalisation and lockout guard.
func (rt *Router) databaseRuleRestriction(w http.ResponseWriter, r *http.Request) (*store.DesiredDatabase, databaseRulesRequest, exposure.Restriction, bool) {
	var req databaseRulesRequest
	var none exposure.Restriction
	if rt.dbAccess == nil || rt.exposure == nil || rt.exposureStore == nil {
		writeError(w, http.StatusNotImplemented, "database network rules are not configured on this control plane")
		return nil, req, none, false
	}
	d, ok := rt.dbAccessDatabase(w, r)
	if !ok {
		return nil, req, none, false
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, errBadBody)
		return nil, req, none, false
	}
	if !d.PubliclyAccessible || d.PublicPort == 0 {
		writeError(w, http.StatusConflict, errNoPublishedPort)
		return nil, req, none, false
	}
	if !rt.isLocalNode(d.NodeID) {
		writeError(w, http.StatusConflict, "rules can only be applied on the control plane's own host in this version")
		return nil, req, none, false
	}
	if len(req.Allow) > maxRuleSources {
		writeError(w, http.StatusBadRequest, "too many sources")
		return nil, req, none, false
	}
	allow := make([]string, 0, len(req.Allow)+1)
	for _, a := range req.Allow {
		if len(a.Description) > maxRuleDescription {
			writeError(w, http.StatusBadRequest, "descriptions are limited to 120 characters")
			return nil, req, none, false
		}
		allow = append(allow, strings.TrimSpace(a.Source))
	}
	if req.LocalContainers {
		allow = append(allow, localContainerSource)
	}
	want, err := rt.exposure.Normalize(exposure.Restriction{Port: d.PublicPort, Protocol: protoTCP, Allow: allow})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, exposure.ErrLockout) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return nil, req, none, false
	}
	return d, req, want, true
}

func noteMap(req databaseRulesRequest, want exposure.Restriction) map[string]string {
	notes := map[string]string{}
	for _, a := range req.Allow {
		d := strings.TrimSpace(a.Description)
		if d == "" {
			continue
		}
		for _, w := range want.Allow {
			if w == strings.TrimSpace(a.Source) || strings.HasPrefix(w, strings.TrimSpace(a.Source)+"/") {
				notes[w] = d
			}
		}
	}
	return notes
}

// handlePreviewDatabaseRules handles POST /api/v1/databases/{name}/network/rules/preview.
func (rt *Router) handlePreviewDatabaseRules(w http.ResponseWriter, r *http.Request) {
	_, req, want, ok := rt.databaseRuleRestriction(w, r)
	if !ok {
		return
	}
	plan, err := rt.exposure.Plan(want)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, databaseRulesPlanResponse{
		exposurePlanResource: exposurePlanResource{Plan: plan, Port: want.Port, Protocol: want.Protocol, Allow: want.Allow, Warnings: rt.exposureWarnings(r, want.Allow)},
		Descriptions:         noteMap(req, want),
	})
}

// handleApplyDatabaseRules handles PUT /api/v1/databases/{name}/network/rules.
func (rt *Router) handleApplyDatabaseRules(w http.ResponseWriter, r *http.Request) {
	d, req, want, ok := rt.databaseRuleRestriction(w, r)
	if !ok {
		return
	}
	plan, err := rt.exposure.Plan(want)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !req.Confirm {
		writeError(w, http.StatusBadRequest, "confirmation required: "+plan.Drops)
		return
	}
	if _, err := rt.exposure.Apply(r.Context(), want); err != nil {
		rt.logger.Error("api: database rules apply failed", slog.String("name", d.Name), slog.Int("port", want.Port), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "could not apply the rule: "+err.Error())
		return
	}
	row := store.ExposureRestriction{Port: want.Port, Protocol: want.Protocol, Allow: want.Allow, CreatedBy: rt.accessActor(r).name, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := rt.exposureStore.SaveExposureRestriction(r.Context(), row); err != nil {
		_ = rt.exposure.Remove(r.Context(), want.Port, want.Protocol)
		rt.internalError(w, "api: database rules: save failed, rule rolled back", err, slog.String("name", d.Name))
		return
	}
	if err := rt.dbAccess.ReplaceDatabaseNetworkRuleNotes(r.Context(), d.Name, noteMap(req, want)); err != nil {
		rt.logger.Warn("api: database rules: notes save failed", slog.String("name", d.Name), slog.String("error", err.Error()))
	}
	rt.auditDatabaseAccess(r, dbaccess.ActionRulesApply, d.Name, "", http.StatusOK)
	rt.logger.Info("api: database network rules applied", slog.String("database", d.Name), slog.Int("port", want.Port), slog.Int("sources", len(want.Allow)))
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, databaseRulesPlanResponse{
		exposurePlanResource: exposurePlanResource{Plan: plan, Port: want.Port, Protocol: want.Protocol, Allow: want.Allow, Warnings: rt.exposureWarnings(r, want.Allow), Applied: true},
		Descriptions:         noteMap(req, want),
	})
}

// handleRemoveDatabaseRules handles DELETE /api/v1/databases/{name}/network/rules.
func (rt *Router) handleRemoveDatabaseRules(w http.ResponseWriter, r *http.Request) {
	if rt.dbAccess == nil || rt.exposure == nil || rt.exposureStore == nil {
		writeError(w, http.StatusNotImplemented, "database network rules are not configured on this control plane")
		return
	}
	d, ok := rt.dbAccessDatabase(w, r)
	if !ok {
		return
	}
	if d.PublicPort != 0 {
		if err := rt.removeRestriction(r.Context(), d.PublicPort); err != nil {
			rt.internalError(w, "api: database rules: remove failed", err, slog.String("name", d.Name))
			return
		}
	}
	if err := rt.dbAccess.ReplaceDatabaseNetworkRuleNotes(r.Context(), d.Name, nil); err != nil {
		rt.logger.Warn("api: database rules: notes clear failed", slog.String("name", d.Name), slog.String("error", err.Error()))
	}
	rt.auditDatabaseAccess(r, dbaccess.ActionRulesRemove, d.Name, "", http.StatusNoContent)
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) removeRestriction(ctx context.Context, port int) error {
	if err := rt.exposureStore.DeleteExposureRestriction(ctx, port, protoTCP); err != nil {
		return err
	}
	if err := rt.exposure.Remove(ctx, port, protoTCP); err != nil {
		rt.logger.Warn("api: database rules: rule removal failed, the reconciler will retry", slog.Int("port", port), slog.String("error", err.Error()))
	}
	return nil
}

// handleMakeDatabasePrivate handles POST /api/v1/databases/{name}/network/make-private:
// stop publishing the port and drop any rule that guarded it.
func (rt *Router) handleMakeDatabasePrivate(w http.ResponseWriter, r *http.Request) {
	if rt.dbAccess == nil {
		writeError(w, http.StatusNotImplemented, errDatabaseAccessOff)
		return
	}
	d, ok := rt.dbAccessDatabase(w, r)
	if !ok {
		return
	}
	if d.PublicPort != 0 && rt.exposure != nil && rt.exposureStore != nil {
		if err := rt.removeRestriction(r.Context(), d.PublicPort); err != nil {
			rt.internalError(w, "api: make database private: rule removal failed", err, slog.String("name", d.Name))
			return
		}
	}
	if _, err := rt.databases.SetDatabasePublicAccess(r.Context(), d.Name, false, 0, ""); err != nil {
		rt.internalError(w, "api: make database private failed", err, slog.String("name", d.Name))
		return
	}
	if err := rt.dbAccess.ReplaceDatabaseNetworkRuleNotes(r.Context(), d.Name, nil); err != nil {
		rt.logger.Warn("api: make database private: notes clear failed", slog.String("name", d.Name), slog.String("error", err.Error()))
	}
	rt.auditDatabaseAccess(r, dbaccess.ActionMakePrivate, d.Name, "", http.StatusNoContent)
	rt.logger.Info("api: database made private", slog.String("database", d.Name))
	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}
