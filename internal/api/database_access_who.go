package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbaccess"
	"github.com/GLINCKER/levelrail/internal/store"
)

type databasePrincipalResource struct {
	Type      string   `json:"type"`
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Effective []string `json:"effective"`
	Via       []string `json:"via"`
	Revoked   bool     `json:"revoked,omitempty"`
}

type databasePolicyRefResource struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Principals  []string `json:"principals"`
	ScopedToOne bool     `json:"scoped_to_database"`
}

type databaseWhoResponse struct {
	Database    string                      `json:"database"`
	Principals  []databasePrincipalResource `json:"principals"`
	Policies    []databasePolicyRefResource `json:"policies"`
	Templates   []string                    `json:"grant_templates"`
	PoliciesURL string                      `json:"policies_path"`
}

const baseAbilitiesVia = "base abilities"

var grantTemplateIDs = []string{"database-read-only", "database-operator", "database-owner"}

// databaseEnvResources returns the environment resources a database adds to
// its own, so environment-scoped policies are evaluated the same way the
// request gate does.
func (rt *Router) databaseEnvResources(ctx context.Context, name string) []string {
	if rt.dbAccess == nil {
		return nil
	}
	ref, err := rt.dbAccess.EnvironmentOfDatabase(ctx, name)
	if err != nil {
		rt.logger.Warn("api: database access: environment lookup failed", slog.String("database", name), slog.String("error", err.Error()))
		return nil
	}
	return environmentResources(ref)
}

func policyTouches(doc *Document, resource string, extra []string) bool {
	targets := append([]string{resource}, extra...)
	for _, s := range doc.Statement {
		for _, pattern := range s.Resource {
			if slices.ContainsFunc(targets, func(t string) bool { return matchesPattern(pattern, t) }) {
				return true
			}
		}
	}
	return false
}

func (rt *Router) principalView(ctx context.Context, typ, id, name string, base []string, resource string, extra []string, revoked bool) (databasePrincipalResource, bool, error) {
	policies, err := rt.policies.ListPoliciesForPrincipal(ctx, typ, id)
	if err != nil {
		return databasePrincipalResource{}, false, fmt.Errorf("list policies for %s %q: %w", typ, id, err)
	}
	var effective, via []string
	for _, ability := range validAbilities {
		if authorizeResource(base, policies, ability, resource, extra...) {
			effective = append(effective, ability)
		}
	}
	if len(effective) == 0 {
		return databasePrincipalResource{}, false, nil
	}
	for _, a := range effective {
		if hasAbility(base, a) {
			via = append(via, baseAbilitiesVia)
			break
		}
	}
	for _, p := range policies {
		doc, perr := ParseDocument(p.Document)
		if perr == nil && policyTouches(doc, resource, extra) {
			via = append(via, "policy "+p.Name)
		}
	}
	return databasePrincipalResource{Type: typ, ID: id, Name: name, Effective: effective, Via: via, Revoked: revoked}, true, nil
}

// handleDatabaseWhoCanAccess handles GET /api/v1/databases/{name}/access/principals.
func (rt *Router) handleDatabaseWhoCanAccess(w http.ResponseWriter, r *http.Request) {
	desired, ok := rt.dbAccessDatabase(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	resource := resourcePrefixDatabase + desired.Name
	extra := rt.databaseEnvResources(ctx, desired.Name)
	resp := databaseWhoResponse{
		Database: desired.Name, Principals: []databasePrincipalResource{}, Policies: []databasePolicyRefResource{},
		Templates: grantTemplateIDs, PoliciesURL: "/access",
	}
	users, err := rt.auth.ListUsers(ctx)
	if err != nil {
		rt.internalError(w, "api: database access: list users failed", err)
		return
	}
	for _, u := range users {
		view, include, err := rt.principalView(ctx, store.PrincipalTypeUser, u.ID, u.DisplayName, u.Abilities, resource, extra, false)
		if err != nil {
			rt.internalError(w, "api: database access: evaluate user failed", err)
			return
		}
		if include {
			resp.Principals = append(resp.Principals, view)
		}
	}
	tokens, err := rt.tokens.ListAPITokens(ctx)
	if err != nil {
		rt.internalError(w, "api: database access: list tokens failed", err)
		return
	}
	for _, t := range tokens {
		if t.RevokedAt != nil || (t.ExpiresAt != nil && t.ExpiresAt.Before(time.Now())) {
			continue
		}
		view, include, err := rt.principalView(ctx, store.PrincipalTypeToken, t.ID, t.Name, t.Abilities, resource, extra, false)
		if err != nil {
			rt.internalError(w, "api: database access: evaluate token failed", err)
			return
		}
		if include {
			resp.Principals = append(resp.Principals, view)
		}
	}
	if err := rt.collectDatabasePolicies(ctx, &resp, resource, extra); err != nil {
		rt.internalError(w, "api: database access: list policies failed", err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (rt *Router) collectDatabasePolicies(ctx context.Context, resp *databaseWhoResponse, resource string, extra []string) error {
	pols, err := rt.policies.ListPolicies(ctx)
	if err != nil {
		return fmt.Errorf("list policies: %w", err)
	}
	for _, p := range pols {
		doc, perr := ParseDocument(p.Document)
		if perr != nil || !policyTouches(doc, resource, extra) {
			continue
		}
		atts, err := rt.policies.ListAttachmentsForPolicy(ctx, p.ID)
		if err != nil {
			return fmt.Errorf("list attachments for policy %q: %w", p.ID, err)
		}
		ref := databasePolicyRefResource{ID: p.ID, Name: p.Name, Principals: []string{}, ScopedToOne: policyOnlyTouches(doc, resource)}
		for _, a := range atts {
			ref.Principals = append(ref.Principals, a.PrincipalType+":"+a.PrincipalID)
		}
		resp.Policies = append(resp.Policies, ref)
	}
	return nil
}

func policyOnlyTouches(doc *Document, resource string) bool {
	for _, s := range doc.Statement {
		for _, r := range s.Resource {
			if r != resource {
				return false
			}
		}
	}
	return true
}

type databaseGrantRequest struct {
	Template      string `json:"template"`
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
	// Preview returns the policy and what it would allow without saving.
	Preview bool `json:"preview"`
}

type databaseGrantResponse struct {
	PolicyName string          `json:"policy_name"`
	Document   json.RawMessage `json:"document"`
	Principal  string          `json:"principal"`
	Applied    bool            `json:"applied"`
	Notes      []string        `json:"notes"`
}

func (rt *Router) grantPrincipalName(ctx context.Context, typ, id string) (string, error) {
	switch typ {
	case store.PrincipalTypeUser:
		u, err := rt.auth.GetUserByID(ctx, id)
		if err != nil {
			return "", fmt.Errorf("user %q: %w", id, err)
		}
		return u.DisplayName, nil
	case store.PrincipalTypeToken:
		t, err := rt.tokens.GetAPITokenByID(ctx, id)
		if err != nil {
			return "", fmt.Errorf("token %q: %w", id, err)
		}
		return t.Name, nil
	}
	return "", errors.New("principal_type must be user or token")
}

// handleGrantDatabaseAccess handles POST /api/v1/databases/{name}/access/grants:
// apply a database scoped IAM template to a user or token, or preview it.
func (rt *Router) handleGrantDatabaseAccess(w http.ResponseWriter, r *http.Request) {
	desired, ok := rt.dbAccessDatabase(w, r)
	if !ok {
		return
	}
	var req databaseGrantRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, errBadBody)
		return
	}
	if !slices.Contains(grantTemplateIDs, req.Template) {
		writeError(w, http.StatusBadRequest, "template must be database-read-only, database-operator or database-owner")
		return
	}
	if !slices.Contains(validPrincipalTypes, req.PrincipalType) || req.PrincipalID == "" {
		writeError(w, http.StatusBadRequest, "principal_type user or token and a principal_id are required")
		return
	}
	pname, err := rt.grantPrincipalName(r.Context(), req.PrincipalType, req.PrincipalID)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) || errors.Is(err, store.ErrAPITokenNotFound) {
			writeError(w, http.StatusNotFound, "principal not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	t, _ := findPolicyTemplate(req.Template)
	params := map[string]string{templateParamDatabase: desired.Name}
	doc, err := json.Marshal(t.build(params))
	if err != nil {
		rt.internalError(w, "api: grant database access: marshal failed", err)
		return
	}
	resp := databaseGrantResponse{
		PolicyName: templatePolicyName(t, params, ""), Document: doc, Principal: pname,
		Notes: []string{
			"Applies to this database only: " + resourcePrefixDatabase + desired.Name + ".",
			"Existing broader abilities of the principal still apply. Denies in this policy win over allows.",
		},
	}
	if req.Preview {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	policyID, err := rt.ensureGrantPolicy(r.Context(), t, resp.PolicyName, string(doc))
	if err != nil {
		rt.internalError(w, "api: grant database access: save policy failed", err, slog.String("database", desired.Name))
		return
	}
	attachID, err := store.NewPolicyAttachmentID()
	if err != nil {
		rt.internalError(w, "api: grant database access: attachment id failed", err)
		return
	}
	if err := rt.policies.AttachPolicy(r.Context(), attachID, policyID, req.PrincipalType, req.PrincipalID); err != nil {
		rt.internalError(w, "api: grant database access: attach failed", err, slog.String("database", desired.Name))
		return
	}
	resp.Applied = true
	rt.auditDatabaseAccess(r, dbaccess.ActionGrantApply, desired.Name, req.PrincipalType+":"+req.PrincipalID+" "+req.Template, http.StatusOK)
	rt.logger.Info("api: database access granted", slog.String("database", desired.Name), slog.String("template", req.Template),
		slog.String("principal_type", req.PrincipalType), slog.String("principal_id", req.PrincipalID))
	writeJSON(w, http.StatusOK, resp)
}

// ensureGrantPolicy returns the id of the named policy, creating it from the
// template document the first time.
func (rt *Router) ensureGrantPolicy(ctx context.Context, t policyTemplate, name, doc string) (string, error) {
	pols, err := rt.policies.ListPolicies(ctx)
	if err != nil {
		return "", fmt.Errorf("list policies: %w", err)
	}
	for _, p := range pols {
		if p.Name == name {
			return p.ID, nil
		}
	}
	id, err := store.NewPolicyID()
	if err != nil {
		return "", fmt.Errorf("policy id: %w", err)
	}
	now := time.Now()
	rec := store.Policy{ID: id, Name: name, Description: t.Description, Document: doc, CreatedAt: now, UpdatedAt: now}
	if err := rt.policies.SavePolicy(ctx, rec); err != nil {
		return "", fmt.Errorf("save policy: %w", err)
	}
	return id, nil
}

// handleRevokeDatabaseGrant handles DELETE
// /api/v1/databases/{name}/access/grants/{policy_id}/{principal_type}/{principal_id}.
func (rt *Router) handleRevokeDatabaseGrant(w http.ResponseWriter, r *http.Request) {
	desired, ok := rt.dbAccessDatabase(w, r)
	if !ok {
		return
	}
	policyID, typ, pid := r.PathValue("policy_id"), r.PathValue("principal_type"), r.PathValue("principal_id")
	if !slices.Contains(validPrincipalTypes, typ) {
		writeError(w, http.StatusBadRequest, "principal_type must be user or token")
		return
	}
	if err := rt.policies.DetachPolicy(r.Context(), policyID, typ, pid); err != nil {
		rt.internalError(w, "api: revoke database grant failed", err, slog.String("database", desired.Name))
		return
	}
	rt.auditDatabaseAccess(r, dbaccess.ActionGrantRemove, desired.Name, typ+":"+pid, http.StatusNoContent)
	w.WriteHeader(http.StatusNoContent)
}
