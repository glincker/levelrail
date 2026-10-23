package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/mailgroups"
	"github.com/GLINCKER/levelrail/internal/store"
)

const mailPreviewSize = 20

type mailGroupRequest struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Rules       mailgroups.Rules `json:"rules"`
}

type mailGroupView struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Rules       mailgroups.Rules `json:"rules"`
	CreatedAt   string           `json:"created_at"`
	UpdatedAt   string           `json:"updated_at"`
	Recipients  int              `json:"recipients"`
	Suppressed  int              `json:"suppressed"`
}

type mailAudiencePreview struct {
	Total       int      `json:"total"`
	Deliverable int      `json:"deliverable"`
	Suppressed  int      `json:"suppressed"`
	Sample      []string `json:"sample"`
}

func decodeGroupRules(g store.MailGroup) mailgroups.Rules {
	var r mailgroups.Rules
	_ = json.Unmarshal(g.Rules, &r)
	return r
}

// audience resolves rules into recipients and the suppressed share, masked for display.
func (rt *Router) audience(rules mailgroups.Rules, dir mailgroups.Directory, suppressed map[string]bool) (send, held []mailgroups.Recipient) {
	return mailgroups.Split(mailgroups.Resolve(dir, rules, time.Now()), suppressed, true)
}

func previewOf(send, held []mailgroups.Recipient) mailAudiencePreview {
	p := mailAudiencePreview{Total: len(send) + len(held), Deliverable: len(send), Suppressed: len(held), Sample: []string{}}
	for i, rc := range send {
		if i >= mailPreviewSize {
			break
		}
		p.Sample = append(p.Sample, mailgroups.Mask(rc.Email))
	}
	return p
}

func (rt *Router) groupView(g store.MailGroup, dir mailgroups.Directory, suppressed map[string]bool) mailGroupView {
	rules := decodeGroupRules(g)
	send, held := rt.audience(rules, dir, suppressed)
	return mailGroupView{ID: g.ID, Name: g.Name, Description: g.Description, Rules: rules, CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt, Recipients: len(send) + len(held), Suppressed: len(held)}
}

// handleListMailGroups handles GET /api/v1/mail/groups, each with its live recipient count.
func (rt *Router) handleListMailGroups(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	ctx := r.Context()
	groups, err := rt.mail.db.ListMailGroups(ctx)
	if err != nil {
		rt.internalError(w, "api: list mail groups failed", err)
		return
	}
	dir, err := rt.mailDirectory(ctx)
	if err != nil {
		rt.internalError(w, "api: mail directory failed", err)
		return
	}
	suppressed, err := rt.mailSuppressedSet(ctx)
	if err != nil {
		rt.internalError(w, "api: list suppressions failed", err)
		return
	}
	out := []mailGroupView{}
	for _, g := range groups {
		out = append(out, rt.groupView(g, dir, suppressed))
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": out})
}

func validateGroupRequest(req *mailGroupRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 80 {
		return errors.New("name is required and at most 80 characters")
	}
	if len(req.Description) > 300 {
		return errors.New("description is too long")
	}
	return req.Rules.Validate()
}

func (rt *Router) saveGroup(w http.ResponseWriter, r *http.Request, id string, created bool) {
	var req mailGroupRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if err := validateGroupRequest(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	g := store.MailGroup{ID: id, Name: req.Name, Description: req.Description, CreatedBy: rt.actorForRequest(r)}
	if existing, err := rt.mail.db.GetMailGroup(ctx, id); err == nil {
		g.CreatedAt, g.CreatedBy = existing.CreatedAt, existing.CreatedBy
	}
	raw, _ := json.Marshal(req.Rules)
	g.Rules = raw
	if err := rt.mail.db.SaveMailGroup(ctx, g); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "a group with that name already exists")
			return
		}
		rt.internalError(w, "api: save mail group failed", err)
		return
	}
	dir, err := rt.mailDirectory(ctx)
	if err != nil {
		rt.internalError(w, "api: mail directory failed", err)
		return
	}
	suppressed, _ := rt.mailSuppressedSet(ctx)
	saved, err := rt.mail.db.GetMailGroup(ctx, id)
	if err != nil {
		rt.internalError(w, "api: reload mail group failed", err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, rt.groupView(*saved, dir, suppressed))
}

// handleCreateMailGroup handles POST /api/v1/mail/groups.
func (rt *Router) handleCreateMailGroup(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	id, err := randomOpaqueID("mg_")
	if err != nil {
		rt.internalError(w, "api: mail group id failed", err)
		return
	}
	rt.saveGroup(w, r, id, true)
}

func (rt *Router) loadMailGroup(w http.ResponseWriter, r *http.Request) (*store.MailGroup, bool) {
	g, err := rt.mail.db.GetMailGroup(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrMailNotFound) {
		writeError(w, http.StatusNotFound, "no such group")
		return nil, false
	}
	if err != nil {
		rt.internalError(w, "api: get mail group failed", err)
		return nil, false
	}
	return g, true
}

// handleGetMailGroup handles GET /api/v1/mail/groups/{id}: the group, its count and a masked first-20 preview.
func (rt *Router) handleGetMailGroup(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	g, ok := rt.loadMailGroup(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	dir, err := rt.mailDirectory(ctx)
	if err != nil {
		rt.internalError(w, "api: mail directory failed", err)
		return
	}
	suppressed, err := rt.mailSuppressedSet(ctx)
	if err != nil {
		rt.internalError(w, "api: list suppressions failed", err)
		return
	}
	send, held := rt.audience(decodeGroupRules(*g), dir, suppressed)
	writeJSON(w, http.StatusOK, map[string]any{"group": rt.groupView(*g, dir, suppressed), "preview": previewOf(send, held)})
}

// handleUpdateMailGroup handles PUT /api/v1/mail/groups/{id}.
func (rt *Router) handleUpdateMailGroup(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	g, ok := rt.loadMailGroup(w, r)
	if !ok {
		return
	}
	rt.saveGroup(w, r, g.ID, false)
}

// handleDeleteMailGroup handles DELETE /api/v1/mail/groups/{id}.
func (rt *Router) handleDeleteMailGroup(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	g, ok := rt.loadMailGroup(w, r)
	if !ok {
		return
	}
	if err := rt.mail.db.DeleteMailGroup(r.Context(), g.ID); err != nil {
		rt.internalError(w, "api: delete mail group failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePreviewMailAudience handles POST /api/v1/mail/audience/preview: live count and masked sample for unsaved rules.
func (rt *Router) handlePreviewMailAudience(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	var req mailGroupRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if err := req.Rules.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	dir, err := rt.mailDirectory(ctx)
	if err != nil {
		rt.internalError(w, "api: mail directory failed", err)
		return
	}
	suppressed, err := rt.mailSuppressedSet(ctx)
	if err != nil {
		rt.internalError(w, "api: list suppressions failed", err)
		return
	}
	send, held := rt.audience(req.Rules, dir, suppressed)
	writeJSON(w, http.StatusOK, previewOf(send, held))
}

// handleMailAudienceOptions handles GET /api/v1/mail/audience-options: what the rule editor can pick from.
func (rt *Router) handleMailAudienceOptions(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	ctx := r.Context()
	teams, err := rt.mail.db.ListOrganizations(ctx)
	if err != nil {
		rt.internalError(w, "api: list teams failed", err)
		return
	}
	projects, err := rt.mail.db.ListProjects(ctx)
	if err != nil {
		rt.internalError(w, "api: list projects failed", err)
		return
	}
	type opt struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		TeamID string `json:"team_id,omitempty"`
	}
	out := map[string]any{"roles": mailgroups.Roles}
	ts, ps := []opt{}, []opt{}
	for _, t := range teams {
		ts = append(ts, opt{ID: t.ID, Name: t.Name})
	}
	for _, p := range projects {
		ps = append(ps, opt{ID: p.ID, Name: p.Name, TeamID: p.OrgID})
	}
	out["teams"], out["projects"] = ts, ps
	writeJSON(w, http.StatusOK, out)
}

// handleListMailSuppressions handles GET /api/v1/mail/suppressions.
func (rt *Router) handleListMailSuppressions(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	list, err := rt.mail.db.ListMailSuppressions(r.Context())
	if err != nil {
		rt.internalError(w, "api: list suppressions failed", err)
		return
	}
	if list == nil {
		list = []store.MailSuppression{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"suppressions": list})
}

type mailSuppressionRequest struct {
	Email  string `json:"email"`
	Reason string `json:"reason"`
}

// handleAddMailSuppression handles POST /api/v1/mail/suppressions.
func (rt *Router) handleAddMailSuppression(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	var req mailSuppressionRequest
	if !decodeReq(w, r, &req) {
		return
	}
	addr := mailgroups.Normalize(req.Email)
	if err := mailgroups.ValidAddress(addr); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "added by an administrator"
	}
	if err := rt.mail.db.AddMailSuppression(r.Context(), addr, reason); err != nil {
		rt.internalError(w, "api: add suppression failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, store.MailSuppression{Email: addr, Reason: reason})
}

// handleRemoveMailSuppression handles DELETE /api/v1/mail/suppressions/{email}.
func (rt *Router) handleRemoveMailSuppression(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	if err := rt.mail.db.RemoveMailSuppression(r.Context(), r.PathValue("email")); err != nil {
		rt.internalError(w, "api: remove suppression failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
