package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	scimUsers  = "Users"
	scimGroups = "Groups"

	auditActionSCIMUserCreated     = "scim.user.created"
	auditActionSCIMUserUpdated     = "scim.user.updated"
	auditActionSCIMUserDeactivated = "scim.user.deactivated"
	auditActionSCIMUserReactivated = "scim.user.reactivated"
	auditActionSCIMRoleChanged     = "scim.user.role_changed"
)

// scimPre is what scimBefore saw, for scimAfter to compare against.
type scimPre struct {
	wasMember    bool
	groupMembers []string
	inactive     bool
	reactivating bool
}

type scimUserBody struct {
	UserName   string          `json:"userName"`
	ExternalID string          `json:"externalId"`
	Active     json.RawMessage `json:"active"`
	Emails     []struct {
		Value   string `json:"value"`
		Primary bool   `json:"primary"`
	} `json:"emails"`
}

// scimInactive reads active as a boolean or as Entra ID's "False" string.
func scimInactive(raw json.RawMessage) bool {
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return !b
	}
	var s string
	return json.Unmarshal(raw, &s) == nil && strings.EqualFold(s, "false")
}

// scimPatchDeactivates reports whether a SCIM PatchOp sets active to false, by path or inside a value object.
func scimPatchDeactivates(body []byte) bool {
	var req struct {
		Operations []struct {
			Path  string          `json:"path"`
			Value json.RawMessage `json:"value"`
		} `json:"Operations"`
	}
	if json.Unmarshal(body, &req) != nil {
		return false
	}
	for _, op := range req.Operations {
		if strings.EqualFold(op.Path, "active") && scimInactive(op.Value) {
			return true
		}
		var obj struct {
			Active json.RawMessage `json:"active"`
		}
		if op.Path == "" && json.Unmarshal(op.Value, &obj) == nil && len(obj.Active) > 0 && scimInactive(obj.Active) {
			return true
		}
	}
	return false
}

func (b scimUserBody) email() string {
	for _, e := range b.Emails {
		if e.Primary && e.Value != "" {
			return strings.ToLower(strings.TrimSpace(e.Value))
		}
	}
	if len(b.Emails) > 0 && b.Emails[0].Value != "" {
		return strings.ToLower(strings.TrimSpace(b.Emails[0].Value))
	}
	return strings.ToLower(strings.TrimSpace(b.UserName))
}

// scimBefore runs the guards that must hold before the library changes
// anything, and snapshots what scimAfter needs. A non-zero status refuses.
func (rt *Router) scimBefore(r *http.Request, c scimCall) (scimPre, int, string) {
	ctx := r.Context()
	var pre scimPre
	switch {
	case c.resource == scimUsers && c.method == http.MethodPost:
		var body scimUserBody
		if json.Unmarshal(c.body, &body) != nil {
			return pre, 0, ""
		}
		pre.inactive = scimInactive(body.Active)
		if err := rt.scimPrepareExisting(ctx, body); err != nil {
			if errors.Is(err, errSCIMRootAccount) {
				return pre, http.StatusConflict, err.Error()
			}
			rt.logger.Error("api: scim: prepare existing account failed", slog.String("error", err.Error()))
			return pre, http.StatusInternalServerError, "internal error"
		}
	case c.resource == scimUsers && c.id != "" && (c.method == http.MethodPatch || c.method == http.MethodDelete):
		du, found, err := rt.authEngine.DirectoryUser(ctx, c.id)
		if err != nil {
			return pre, http.StatusInternalServerError, "internal error"
		}
		pre.wasMember = found && du.Member
		if c.method == http.MethodPatch && found && !du.Member && scimPatchActivates(c.body) {
			if err := rt.authEngine.SetDirectoryMember(ctx, c.id, true); err != nil {
				rt.logger.Error("api: scim: reactivate membership failed", slog.String("engine_user_id", c.id), slog.String("error", err.Error()))
				return pre, http.StatusInternalServerError, "internal error"
			}
			pre.reactivating = true
		}
		if pre.wasMember && (c.method == http.MethodDelete || scimPatchDeactivates(c.body)) {
			if status, detail := rt.scimLastRootGuard(ctx, c.id); status != 0 {
				return pre, status, detail
			}
		}
	case c.resource == scimGroups && c.id != "" && (c.method == http.MethodPatch || c.method == http.MethodDelete):
		ids, err := rt.authEngine.GroupMemberIDs(ctx, c.id)
		if err != nil {
			return pre, http.StatusInternalServerError, "internal error"
		}
		pre.groupMembers = ids
	}
	return pre, 0, ""
}

// errSCIMRootAccount keeps directory sync away from accounts that hold root.
var errSCIMRootAccount = errors.New("accounts with root access are not managed by directory provisioning")

// scimPrepareExisting mirrors a platform account with the posted email into
// the library and binds it to the posted externalId, so the library's own
// idempotent create adopts it instead of answering 409.
func (rt *Router) scimPrepareExisting(ctx context.Context, body scimUserBody) error {
	email := body.email()
	if email == "" {
		return nil
	}
	existing, err := rt.auth.GetUserByEmail(ctx, email)
	if errors.Is(err, store.ErrUserNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if slices.Contains(existing.Abilities, AbilityRoot) {
		return errSCIMRootAccount
	}
	engine, err := rt.libSessions.SyncUser(ctx, toLibUser(*existing))
	if err != nil {
		return err
	}
	_, err = rt.authEngine.AdoptForSCIM(ctx, engine, body.ExternalID)
	return err
}

func (rt *Router) scimLastRootGuard(ctx context.Context, engineID string) (int, string) {
	legacy, err := rt.authEngine.LegacyUserFor(ctx, engineID)
	if err != nil {
		return http.StatusInternalServerError, "internal error"
	}
	if legacy == "" {
		return 0, ""
	}
	if err := rt.guardLastRoot(ctx, legacy); err != nil {
		if errors.Is(err, errLastRoot) {
			return http.StatusConflict, err.Error()
		}
		return http.StatusInternalServerError, "internal error"
	}
	// Any root member, not only the last one, stays out of directory control.
	user, err := rt.auth.GetUserByID(ctx, legacy)
	if err != nil {
		return http.StatusInternalServerError, "internal error"
	}
	if slices.Contains(user.Abilities, AbilityRoot) {
		return http.StatusConflict, errSCIMRootAccount.Error()
	}
	return 0, ""
}

// scimAfter mirrors a change the library applied onto platform users. A
// non-zero status means the platform could not follow; what it could undo it did.
func (rt *Router) scimAfter(r *http.Request, c scimCall, pre scimPre, rec *scimCapture) (int, string) {
	ctx := r.Context()
	switch {
	case c.resource == scimUsers && c.method == http.MethodPost:
		id, err := scimResponseID(rec.body.Bytes())
		if err != nil {
			return http.StatusInternalServerError, "internal error"
		}
		return rt.scimSettle(r, c, id, scimSettleOpts{created: rec.status == http.StatusCreated, reactivate: true, deactivate: pre.inactive})
	case c.resource == scimUsers && c.id != "" && (c.method == http.MethodPatch || c.method == http.MethodDelete):
		du, found, err := rt.authEngine.DirectoryUser(ctx, c.id)
		if err != nil {
			return http.StatusInternalServerError, "internal error"
		}
		if pre.wasMember && (!found || !du.Member) {
			if status, detail := rt.scimLastRootGuard(ctx, c.id); status != 0 {
				if found {
					if rerr := rt.authEngine.SetDirectoryMember(ctx, c.id, true); rerr != nil {
						rt.logger.Error("api: scim: restore last root membership failed", slog.String("error", rerr.Error()))
					}
				}
				return status, detail
			}
		}
		return rt.scimSettle(r, c, c.id, scimSettleOpts{reactivate: pre.reactivating})
	case c.resource == scimGroups && c.method != http.MethodGet:
		ids := pre.groupMembers
		gid := c.id
		if c.method == http.MethodPost {
			id, err := scimResponseID(rec.body.Bytes())
			if err != nil {
				return http.StatusInternalServerError, "internal error"
			}
			gid = id
		}
		if c.method != http.MethodDelete {
			after, err := rt.authEngine.GroupMemberIDs(ctx, gid)
			if err != nil {
				return http.StatusInternalServerError, "internal error"
			}
			ids = append(ids, after...)
		}
		if err := rt.scimSyncRoles(r, c.tokenID, ids); err != nil {
			rt.logger.Error("api: scim: sync directory roles failed", slog.String("error", err.Error()))
			return http.StatusInternalServerError, "internal error"
		}
	}
	return 0, ""
}

type scimSettleOpts struct {
	created    bool
	reactivate bool
	deactivate bool
}

// scimSettle makes the platform user match the library's record: a member
// gets an account (created on first sight) and its mapped role; a non-member
// is disabled, which revokes its sessions and tokens in the library.
func (rt *Router) scimSettle(r *http.Request, c scimCall, engineID string, o scimSettleOpts) (int, string) {
	ctx := r.Context()
	createdLegacy := ""
	// A user this call created is removed again on failure, so the directory's retry starts clean.
	fail := func(err error) (int, string) {
		rt.logger.Error("api: scim: settle platform user failed", slog.String("engine_user_id", engineID), slog.String("error", err.Error()))
		var derr error
		switch {
		case createdLegacy != "":
			derr = rt.auth.DeleteUser(context.WithoutCancel(ctx), createdLegacy)
		case o.created:
			derr = rt.authEngine.DiscardLibraryUser(context.WithoutCancel(ctx), engineID)
		}
		if derr != nil {
			rt.logger.Error("api: scim: roll back created user failed", slog.String("engine_user_id", engineID), slog.String("error", derr.Error()))
		}
		return http.StatusInternalServerError, "internal error"
	}
	du, found, err := rt.authEngine.DirectoryUser(ctx, engineID)
	if err != nil {
		return fail(err)
	}
	legacy, err := rt.authEngine.LegacyUserFor(ctx, engineID)
	if err != nil {
		return fail(err)
	}
	if o.deactivate && found && du.Member {
		// A user created inactive still gets its account, disabled from the start.
		if legacy == "" {
			if legacy, err = rt.scimProvision(ctx, du); err != nil {
				return fail(err)
			}
			createdLegacy = legacy
		}
		if err := rt.authEngine.SetDirectoryMember(ctx, engineID, false); err != nil {
			return fail(err)
		}
		du.Member = false
	}
	if !found || !du.Member {
		if legacy == "" {
			return 0, ""
		}
		if err := rt.scimDeprovision(r, c.tokenID, legacy); err != nil {
			// Put the membership back so the directory's retry runs the deactivation again.
			if found && createdLegacy == "" {
				if rerr := rt.authEngine.SetDirectoryMember(context.WithoutCancel(ctx), engineID, true); rerr != nil {
					rt.logger.Error("api: scim: restore membership after failed deactivation failed", slog.String("engine_user_id", engineID), slog.String("error", rerr.Error()))
				}
			}
			return fail(err)
		}
		return 0, ""
	}
	if legacy == "" {
		if createdLegacy, err = rt.scimProvision(ctx, du); err != nil {
			return fail(err)
		}
		rt.auditSCIM(r, c.tokenID, auditActionSCIMUserCreated, "/Users/"+engineID, http.StatusCreated)
	} else if o.reactivate && rt.accountDisabled(ctx, legacy) {
		if err := rt.authEngine.EnableUser(ctx, legacy); err != nil {
			return fail(err)
		}
		rt.auditSCIM(r, c.tokenID, auditActionSCIMUserReactivated, "/Users/"+engineID, http.StatusOK)
	} else {
		rt.auditSCIM(r, c.tokenID, auditActionSCIMUserUpdated, "/Users/"+engineID, http.StatusOK)
	}
	if err := rt.scimSyncRoles(r, c.tokenID, []string{engineID}); err != nil {
		return fail(err)
	}
	return 0, ""
}

// scimProvision creates the platform account for a directory user and maps it
// onto the library user the directory created.
func (rt *Router) scimProvision(ctx context.Context, du authengine.DirectoryUser) (string, error) {
	s, err := rt.sso.GetSSOSettings(ctx)
	if err != nil {
		return "", err
	}
	mappings, err := rt.sso.ListSSOGroupMappings(ctx)
	if err != nil {
		return "", err
	}
	role, ok := resolveSSORole(du.Groups, mappings)
	if !ok {
		role = s.SCIMDefaultRoleID
	}
	if role == "" {
		return "", errors.New("no role for directory user: set a SCIM default role")
	}
	id, err := randomOpaqueID("user_")
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(du.DisplayName)
	if name == "" {
		name = du.Email
	}
	if err := rt.sso.CreateProvisionedUser(ctx, store.User{ID: id, Email: du.Email, DisplayName: name, CreatedAt: time.Now()}, role); err != nil {
		return "", fmt.Errorf("provision directory user: %w", err)
	}
	if err := rt.libSessions.LinkUser(ctx, id, du.EngineID); err != nil {
		if derr := rt.auth.DeleteUser(context.WithoutCancel(ctx), id); derr != nil {
			rt.logger.Error("api: scim: roll back platform user failed", slog.String("user_id", id), slog.String("error", derr.Error()))
		}
		return "", err
	}
	return id, nil
}

// scimDeprovision disables the account in the library, which revokes its
// sessions, API tokens and trusted devices in the same call.
func (rt *Router) scimDeprovision(r *http.Request, tokenID, legacy string) error {
	ctx := r.Context()
	if rt.accountDisabled(ctx, legacy) {
		return nil
	}
	if err := rt.authEngine.DisableUser(ctx, legacy); err != nil {
		return err
	}
	rt.sessions.revokeAll(legacy)
	rt.revokeTrustedDevices(ctx, r, signInActor{kind: auditActorTypeSCIM, id: tokenID, name: tokenID}, legacy, "scim_deprovisioned")
	rt.offboardDisabledUser(r, legacy)
	rt.auditSCIM(r, tokenID, auditActionSCIMUserDeactivated, "/Users/"+legacy, http.StatusOK)
	return nil
}

// scimSyncRoles reapplies group mappings to directory users. With no mappings
// configured, roles stay under manual control.
func (rt *Router) scimSyncRoles(r *http.Request, tokenID string, engineIDs []string) error {
	ctx := r.Context()
	mappings, err := rt.sso.ListSSOGroupMappings(ctx)
	if err != nil || len(mappings) == 0 {
		return err
	}
	s, err := rt.sso.GetSSOSettings(ctx)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, id := range engineIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		if err := rt.scimSyncRole(r, tokenID, id, mappings, s.SCIMDefaultRoleID); err != nil {
			return err
		}
	}
	return nil
}

func (rt *Router) scimSyncRole(r *http.Request, tokenID, engineID string, mappings []store.SSOGroupMapping, fallback string) error {
	ctx := r.Context()
	du, found, err := rt.authEngine.DirectoryUser(ctx, engineID)
	if err != nil || !found || !du.Member {
		return err
	}
	legacy, err := rt.authEngine.LegacyUserFor(ctx, engineID)
	if err != nil || legacy == "" {
		return err
	}
	role, ok := resolveSSORole(du.Groups, mappings)
	if !ok {
		role = fallback
	}
	if role == "" {
		return nil
	}
	user, err := rt.auth.GetUserByID(ctx, legacy)
	if err != nil {
		return err
	}
	before, err := rt.roles.UserRoleID(ctx, legacy)
	if err != nil {
		return err
	}
	if _, err := rt.applySSORole(ctx, *user, role); err != nil {
		return err
	}
	if after, _ := rt.roles.UserRoleID(ctx, legacy); after != before {
		rt.auditSCIM(r, tokenID, auditActionSCIMRoleChanged, "/Users/"+engineID+"?role="+after, http.StatusOK)
	}
	return nil
}
