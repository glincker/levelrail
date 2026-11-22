package api

import (
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type cloudGrantWire struct {
	ID            string     `json:"id"`
	AccountID     string     `json:"account_id"`
	ResourceKind  string     `json:"resource_kind"`
	ResourceRef   string     `json:"resource_ref"`
	PrincipalType string     `json:"principal_type"`
	PrincipalID   string     `json:"principal_id"`
	Actions       []string   `json:"actions"`
	GrantedBy     string     `json:"granted_by"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

func toCloudGrantWire(g store.CloudGrant) cloudGrantWire {
	return cloudGrantWire{ID: g.ID, AccountID: g.AccountID, ResourceKind: g.ResourceKind, ResourceRef: g.ResourceRef, PrincipalType: g.PrincipalType,
		PrincipalID: g.PrincipalID, Actions: g.Actions, GrantedBy: g.GrantedBy, ExpiresAt: g.ExpiresAt, CreatedAt: g.CreatedAt}
}

type cloudGrantBody struct {
	ResourceKind  string     `json:"resource_kind,omitempty"`
	ResourceRef   string     `json:"resource_ref,omitempty"`
	PrincipalType string     `json:"principal_type,omitempty"`
	PrincipalID   string     `json:"principal_id,omitempty"`
	Actions       []string   `json:"actions,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

func grantResource(g store.CloudGrant) cloudResource {
	switch g.ResourceKind {
	case store.CloudResourceCompartment:
		return cloudCompartmentResource(g.AccountID, g.ResourceRef)
	case store.CloudResourceInstance:
		return cloudInstanceResource(g.AccountID, "", g.ResourceRef)
	}
	return cloudAccountResource(g.AccountID)
}

func grantDetail(g store.CloudGrant) map[string]any {
	d := map[string]any{"account": g.AccountID, "scope": grantResource(g).String(), "principal_type": g.PrincipalType, "principal_id": g.PrincipalID, "actions": g.Actions}
	if g.ExpiresAt != nil {
		d["expires_at"] = g.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return d
}

// authorizeGrantChange checks the caller may hand out g: root, owner team admin, or a share holder with no escalation.
// prior is the stored row being replaced or removed, nil for a new one.
func (rt *Router) authorizeGrantChange(r *http.Request, st cloudAccountStore, acct store.CloudAccount, g store.CloudGrant, prior *store.CloudGrant) (grantor string, err error) {
	ptype, pid, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		return "", err
	}
	root := hasAbility(abilities, AbilityRoot)
	if root {
		if root, err = rt.tokenScopeAllowsAccount(r.Context(), ptype, pid, acct); err != nil {
			return "", err
		}
	}
	if prior != nil && !root && strings.HasPrefix(prior.GrantedBy, store.CloudGrantRootPrefix) {
		return "", errors.New("only root can change a grant that root issued")
	}
	caller, err := rt.cloudCallerFor(r.Context(), st, ptype, pid)
	if err != nil {
		return "", err
	}
	userID := pid
	if len(caller.principals) > 0 {
		userID = caller.principals[0].ID
	}
	grants, err := st.ListCloudGrantsForPrincipals(r.Context(), acct.ID, caller.principals)
	if err != nil {
		return "", err
	}
	err = authorizeCloudGrant(cloudGrantRequest{Now: time.Now().UTC(), Root: root, Resource: grantResource(g), Actions: g.Actions,
		AccountInstanceScoped: acct.TeamID == "", OwnerTeamAdmin: acct.TeamID != "" && isTeamAdminRole(caller.teamRoles[acct.TeamID]),
		GrantorGrants: grants, ExpiresAt: g.ExpiresAt})
	if root {
		return store.CloudGrantRootPrefix + userID, err
	}
	return userID, err
}

func (rt *Router) handleListCloudGrants(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	st, _ := rt.cloudAcctStore()
	grants, err := st.ListCloudGrants(r.Context(), a.ID)
	if err != nil {
		rt.internalError(w, "api: list cloud grants failed", err, slog.String("account", a.ID))
		return
	}
	out := make([]cloudGrantWire, 0, len(grants))
	for _, g := range grants {
		out = append(out, toCloudGrantWire(g))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleCreateCloudGrant(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	st, _ := rt.cloudAcctStore()
	var body cloudGrantBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.ResourceKind == "" {
		body.ResourceKind = store.CloudResourceAccount
	}
	id, err := newCloudID("cg_")
	if err != nil {
		rt.internalError(w, "api: mint cloud grant id failed", err)
		return
	}
	g := store.CloudGrant{ID: id, AccountID: a.ID, ResourceKind: body.ResourceKind, ResourceRef: body.ResourceRef, PrincipalType: body.PrincipalType,
		PrincipalID: body.PrincipalID, Actions: body.Actions, ExpiresAt: body.ExpiresAt}
	if err := store.ValidateCloudGrant(g); err != nil {
		writeError(w, http.StatusBadRequest, "the grant is not valid: check scope, principal and actions")
		return
	}
	if g.ExpiresAt != nil && !g.ExpiresAt.After(time.Now()) {
		writeError(w, http.StatusBadRequest, "expires_at must be in the future")
		return
	}
	if g.PrincipalType == store.CloudPrincipalUser {
		if _, err := rt.auth.GetUserByID(r.Context(), g.PrincipalID); err != nil {
			writeError(w, http.StatusBadRequest, "unknown user")
			return
		}
	}
	existing, err := rt.conflictingCloudGrant(r, st, g)
	if err != nil {
		rt.internalError(w, "api: list cloud grants failed", err, slog.String("account", a.ID))
		return
	}
	grantor, err := rt.authorizeGrantChange(r, st, a, g, existing)
	if err != nil {
		rt.writeGrantRefusal(w, err)
		return
	}
	g.GrantedBy = grantor
	if err := st.UpsertCloudGrant(r.Context(), g); err != nil {
		rt.internalError(w, "api: save cloud grant failed", err, slog.String("account", a.ID))
		return
	}
	rt.auditCloud(r, "cloud.grant.created", grantDetail(g))
	writeJSON(w, http.StatusCreated, toCloudGrantWire(g))
}

// conflictingCloudGrant finds the stored row an upsert of g would replace.
func (rt *Router) conflictingCloudGrant(r *http.Request, st cloudAccountStore, g store.CloudGrant) (*store.CloudGrant, error) {
	grants, err := st.ListCloudGrants(r.Context(), g.AccountID)
	if err != nil {
		return nil, err
	}
	for i := range grants {
		o := grants[i]
		if o.ResourceKind == g.ResourceKind && o.ResourceRef == g.ResourceRef && o.PrincipalType == g.PrincipalType && o.PrincipalID == g.PrincipalID {
			return &o, nil
		}
	}
	return nil, nil
}

func (rt *Router) writeGrantRefusal(w http.ResponseWriter, err error) {
	var msg string
	switch {
	case errors.Is(err, errCloudNoEscalation):
		msg = err.Error()
	case err != nil && err.Error() != "":
		msg = err.Error()
	}
	if msg == "" || errors.Is(err, errCloudStoreMissing) {
		rt.internalError(w, "api: authorize cloud grant failed", err)
		return
	}
	writeError(w, http.StatusForbidden, msg)
}

func (rt *Router) findCloudGrant(w http.ResponseWriter, r *http.Request, st cloudAccountStore, accountID string) (store.CloudGrant, bool) {
	grants, err := st.ListCloudGrants(r.Context(), accountID)
	if err != nil {
		rt.internalError(w, "api: list cloud grants failed", err, slog.String("account", accountID))
		return store.CloudGrant{}, false
	}
	gid := r.PathValue("gid")
	if i := slices.IndexFunc(grants, func(g store.CloudGrant) bool { return g.ID == gid }); i >= 0 {
		return grants[i], true
	}
	writeError(w, http.StatusNotFound, "cloud grant not found")
	return store.CloudGrant{}, false
}

func (rt *Router) handleUpdateCloudGrant(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	st, _ := rt.cloudAcctStore()
	g, ok := rt.findCloudGrant(w, r, st, a.ID)
	if !ok {
		return
	}
	prior := g
	var body cloudGrantBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.Actions) > 0 {
		g.Actions = body.Actions
	}
	if body.ExpiresAt != nil {
		g.ExpiresAt = body.ExpiresAt
	}
	if err := store.ValidateCloudGrant(g); err != nil {
		writeError(w, http.StatusBadRequest, "the grant is not valid: check actions")
		return
	}
	if body.ExpiresAt != nil && !body.ExpiresAt.After(time.Now()) {
		writeError(w, http.StatusBadRequest, "expires_at must be in the future")
		return
	}
	grantor, err := rt.authorizeGrantChange(r, st, a, g, &prior)
	if err != nil {
		rt.writeGrantRefusal(w, err)
		return
	}
	g.GrantedBy = grantor
	if err := st.UpsertCloudGrant(r.Context(), g); err != nil {
		rt.internalError(w, "api: update cloud grant failed", err, slog.String("account", a.ID))
		return
	}
	rt.auditCloud(r, "cloud.grant.updated", grantDetail(g))
	writeJSON(w, http.StatusOK, toCloudGrantWire(g))
}

func (rt *Router) handleDeleteCloudGrant(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	st, _ := rt.cloudAcctStore()
	g, ok := rt.findCloudGrant(w, r, st, a.ID)
	if !ok {
		return
	}
	if _, err := rt.authorizeGrantChange(r, st, a, g, &g); err != nil {
		rt.writeGrantRefusal(w, err)
		return
	}
	if err := st.DeleteCloudGrant(r.Context(), g.ID); err != nil {
		rt.internalError(w, "api: delete cloud grant failed", err, slog.String("account", a.ID))
		return
	}
	rt.auditCloud(r, "cloud.grant.deleted", grantDetail(g))
	w.WriteHeader(http.StatusNoContent)
}
