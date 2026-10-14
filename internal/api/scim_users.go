package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/scim"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	scimTypeInvalidValue  = "invalidValue"
	scimTypeInvalidFilter = "invalidFilter"
	scimTypeUniqueness    = "uniqueness"
	scimTypeMutability    = "mutability"
)

func (rt *Router) scimUserResource(r *http.Request, u store.SCIMUser) (scim.User, error) {
	refs, err := rt.scim.SCIMGroupRefsForUser(r.Context(), u.UserID)
	if err != nil {
		return scim.User{}, err
	}
	active := !u.Disabled
	out := scim.User{
		Schemas: []string{scim.SchemaUser}, ID: u.UserID, ExternalID: u.ExternalID, UserName: u.UserName,
		DisplayName: u.DisplayName, Active: &active,
		Name:   &scim.Name{Formatted: u.DisplayName, GivenName: u.GivenName, FamilyName: u.FamilyName},
		Emails: []scim.Email{{Value: u.Email, Type: "work", Primary: true}},
		Meta: &scim.Meta{ResourceType: "User", Created: u.CreatedAt.UTC().Format(time.RFC3339),
			LastModified: u.UpdatedAt.UTC().Format(time.RFC3339), Location: rt.scimBase(r.Context()) + "/Users/" + u.UserID},
	}
	for _, ref := range refs {
		out.Groups = append(out.Groups, scim.Ref{Value: ref[0], Display: ref[1]})
	}
	return out, nil
}

func (rt *Router) writeSCIMUser(w http.ResponseWriter, r *http.Request, status int, userID string) {
	u, err := rt.scim.GetSCIMUser(r.Context(), userID)
	if err != nil {
		rt.scimStoreError(w, "load user", err)
		return
	}
	res, err := rt.scimUserResource(r, u)
	if err != nil {
		rt.scimStoreError(w, "load user groups", err)
		return
	}
	writeSCIM(w, status, res)
}

func (rt *Router) scimStoreError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, store.ErrUserNotFound), errors.Is(err, store.ErrSCIMGroupNotFound):
		writeSCIMError(w, http.StatusNotFound, "", "resource not found")
	case errors.Is(err, store.ErrSCIMGroupNameTaken), errors.Is(err, store.ErrUserEmailExists):
		writeSCIMError(w, http.StatusConflict, scimTypeUniqueness, "a resource with that name already exists")
	case errors.Is(err, store.ErrSCIMMemberUnknown):
		writeSCIMError(w, http.StatusBadRequest, scimTypeInvalidValue, "a member is not a known user")
	case errors.Is(err, errLastRoot):
		writeSCIMError(w, http.StatusConflict, scimTypeMutability, err.Error())
	default:
		rt.logger.Error("api: scim "+op+" failed", slog.String("error", err.Error()))
		writeSCIMError(w, http.StatusInternalServerError, "", "internal error")
	}
}

func (rt *Router) handleSCIMListUsers(w http.ResponseWriter, r *http.Request, _ store.SCIMToken) {
	f, err := scim.ParseFilter(r.URL.Query().Get("filter"), store.SCIMFilterUserName, store.SCIMFilterExternalID, store.SCIMFilterEmail, store.SCIMFilterID)
	if err != nil {
		writeSCIMError(w, http.StatusBadRequest, scimTypeInvalidFilter, "supported: userName, externalId, emails.value or id with eq")
		return
	}
	attr, value := "", ""
	if f != nil {
		attr, value = f.Attribute, f.Value
	}
	offset, limit, start := scimPage(r)
	users, total, err := rt.scim.ListSCIMUsers(r.Context(), attr, value, offset, limit)
	if err != nil {
		rt.scimStoreError(w, "list users", err)
		return
	}
	out := scim.UserList{ListHeader: scim.NewListHeader(len(users), total, start), Resources: []scim.User{}}
	for _, u := range users {
		res, err := rt.scimUserResource(r, u)
		if err != nil {
			rt.scimStoreError(w, "list users", err)
			return
		}
		out.Resources = append(out.Resources, res)
	}
	writeSCIM(w, http.StatusOK, out)
}

func (rt *Router) handleSCIMGetUser(w http.ResponseWriter, r *http.Request, _ store.SCIMToken) {
	rt.writeSCIMUser(w, r, http.StatusOK, r.PathValue("id"))
}

func normalizeSCIMEmail(raw string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	addr, err := mail.ParseAddress(email)
	return email, err == nil && addr.Address == email
}

// handleSCIMCreateUser provisions an account with the directory default role.
// A user the directory deleted earlier is restored instead of duplicated.
func (rt *Router) handleSCIMCreateUser(w http.ResponseWriter, r *http.Request, tok store.SCIMToken) {
	var in scim.User
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.UserName) == "" {
		writeSCIMError(w, http.StatusBadRequest, scimTypeInvalidValue, "userName is required")
		return
	}
	email, ok := normalizeSCIMEmail(in.PrimaryEmail())
	if !ok {
		writeSCIMError(w, http.StatusBadRequest, scimTypeInvalidValue, "an email address is required in emails or userName")
		return
	}
	ctx := r.Context()
	now := time.Now()
	su := store.SCIMUser{ExternalID: in.ExternalID, UserName: strings.TrimSpace(in.UserName), DisplayName: scimDisplayName(in, email)}
	if in.Name != nil {
		su.GivenName, su.FamilyName = in.Name.GivenName, in.Name.FamilyName
	}
	active := in.Active == nil || *in.Active
	if existing, err := rt.auth.GetUserByEmail(ctx, email); err == nil {
		rt.scimRestoreUser(w, r, tok, existing.ID, su, active)
		return
	} else if !errors.Is(err, store.ErrUserNotFound) {
		rt.scimStoreError(w, "create user", err)
		return
	}
	role, err := rt.scimDefaultRole(ctx)
	if err != nil {
		rt.scimStoreError(w, "create user", err)
		return
	}
	id, err := randomOpaqueID("user_")
	if err != nil {
		rt.scimStoreError(w, "create user", err)
		return
	}
	err = rt.sso.CreateProvisionedUser(ctx, store.ProvisionedUser{
		User: store.User{ID: id, Email: email, DisplayName: su.DisplayName, CreatedAt: now}, RoleID: role, SCIM: &su, Disabled: !active,
	})
	if err != nil {
		rt.scimStoreError(w, "create user", err)
		return
	}
	rt.auditSCIM(r, tok, auditActionSCIMUserCreated, "/Users/"+id, http.StatusCreated)
	rt.writeSCIMUser(w, r, http.StatusCreated, id)
}

// scimRestoreUser answers a create for an email that already has an account:
// 409 when the directory can still see it, otherwise it is adopted back.
func (rt *Router) scimRestoreUser(w http.ResponseWriter, r *http.Request, tok store.SCIMToken, userID string, su store.SCIMUser, active bool) {
	if _, err := rt.scim.GetSCIMUser(r.Context(), userID); err == nil {
		writeSCIMError(w, http.StatusConflict, scimTypeUniqueness, "a user with that email already exists")
		return
	}
	su.UserID = userID
	if err := rt.scim.SaveSCIMUser(r.Context(), su, time.Now()); err != nil {
		rt.scimStoreError(w, "restore user", err)
		return
	}
	if err := rt.scimSetActive(r, tok, userID, true, active); err != nil {
		rt.scimStoreError(w, "restore user", err)
		return
	}
	rt.auditSCIM(r, tok, auditActionSCIMUserCreated, "/Users/"+userID+"?restored=true", http.StatusCreated)
	rt.writeSCIMUser(w, r, http.StatusCreated, userID)
}

func scimDisplayName(in scim.User, email string) string {
	switch {
	case strings.TrimSpace(in.DisplayName) != "":
		return strings.TrimSpace(in.DisplayName)
	case in.Name != nil && strings.TrimSpace(in.Name.Formatted) != "":
		return strings.TrimSpace(in.Name.Formatted)
	case in.Name != nil && strings.TrimSpace(in.Name.GivenName+" "+in.Name.FamilyName) != "":
		return strings.TrimSpace(in.Name.GivenName + " " + in.Name.FamilyName)
	}
	return email
}

// scimSetActive moves a user between active and disabled. Deactivation runs
// the full deprovision; the last root user is never taken out.
func (rt *Router) scimSetActive(r *http.Request, tok store.SCIMToken, userID string, wasDisabled, active bool) error {
	switch {
	case !active && !wasDisabled:
		if err := rt.guardLastRoot(r.Context(), userID); err != nil {
			return err
		}
		return rt.deprovisionUser(r, tok, userID, false)
	case active && wasDisabled:
		return rt.reactivateUser(r, tok, userID)
	}
	return nil
}

func (rt *Router) handleSCIMReplaceUser(w http.ResponseWriter, r *http.Request, tok store.SCIMToken) {
	var in scim.User
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.UserName) == "" {
		writeSCIMError(w, http.StatusBadRequest, scimTypeInvalidValue, "userName is required")
		return
	}
	change := scim.UserChange{Active: in.Active}
	userName, display, ext := strings.TrimSpace(in.UserName), strings.TrimSpace(in.DisplayName), in.ExternalID
	change.UserName, change.DisplayName, change.ExternalID = &userName, &display, &ext
	if in.Name != nil {
		change.GivenName, change.FamilyName = &in.Name.GivenName, &in.Name.FamilyName
	}
	rt.scimApplyUserChange(w, r, tok, change)
}

func (rt *Router) handleSCIMPatchUser(w http.ResponseWriter, r *http.Request, tok store.SCIMToken) {
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeSCIMError(w, http.StatusBadRequest, scimTypeInvalidValue, "invalid body")
		return
	}
	change, err := scim.ParseUserPatch(raw)
	if err != nil {
		writeSCIMError(w, http.StatusBadRequest, scimTypeInvalidValue, err.Error())
		return
	}
	rt.scimApplyUserChange(w, r, tok, change)
}

// scimApplyUserChange runs the activation change before the profile write, so
// a failed save can never leave a deactivated user still signed in.
func (rt *Router) scimApplyUserChange(w http.ResponseWriter, r *http.Request, tok store.SCIMToken, c scim.UserChange) {
	cur, err := rt.scim.GetSCIMUser(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.scimStoreError(w, "update user", err)
		return
	}
	if c.Active != nil {
		if err := rt.scimSetActive(r, tok, cur.UserID, cur.Disabled, *c.Active); err != nil {
			rt.scimStoreError(w, "update user", err)
			return
		}
	}
	next := cur
	for dst, src := range map[*string]*string{&next.UserName: c.UserName, &next.DisplayName: c.DisplayName,
		&next.ExternalID: c.ExternalID, &next.GivenName: c.GivenName, &next.FamilyName: c.FamilyName} {
		if src != nil {
			*dst = *src
		}
	}
	if next.UserName == "" {
		next.UserName = cur.Email
	}
	if next.DisplayName == "" {
		next.DisplayName = cur.Email
	}
	if next != cur {
		if err := rt.scim.SaveSCIMUser(r.Context(), next, time.Now()); err != nil {
			rt.scimStoreError(w, "update user", err)
			return
		}
		rt.auditSCIM(r, tok, auditActionSCIMUserUpdated, "/Users/"+cur.UserID, http.StatusOK)
	}
	rt.writeSCIMUser(w, r, http.StatusOK, cur.UserID)
}

func (rt *Router) handleSCIMDeleteUser(w http.ResponseWriter, r *http.Request, tok store.SCIMToken) {
	id := r.PathValue("id")
	if _, err := rt.scim.GetSCIMUser(r.Context(), id); err != nil {
		rt.scimStoreError(w, "delete user", err)
		return
	}
	if err := rt.guardLastRoot(r.Context(), id); err != nil {
		rt.scimStoreError(w, "delete user", err)
		return
	}
	if err := rt.deprovisionUser(r, tok, id, true); err != nil {
		rt.scimStoreError(w, "delete user", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
