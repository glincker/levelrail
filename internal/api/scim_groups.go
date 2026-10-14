package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/scim"
	"github.com/GLINCKER/levelrail/internal/store"
)

func (rt *Router) scimGroupResource(r *http.Request, g store.SCIMGroup) scim.Group {
	out := scim.Group{
		Schemas: []string{scim.SchemaGroup}, ID: g.ID, ExternalID: g.ExternalID, DisplayName: g.DisplayName,
		Members: []scim.Ref{},
		Meta: &scim.Meta{ResourceType: "Group", Created: g.CreatedAt.UTC().Format(time.RFC3339),
			LastModified: g.UpdatedAt.UTC().Format(time.RFC3339), Location: rt.scimBase(r.Context()) + "/Groups/" + g.ID},
	}
	for _, id := range g.Members {
		out.Members = append(out.Members, scim.Ref{Value: id})
	}
	return out
}

func (rt *Router) writeSCIMGroup(w http.ResponseWriter, r *http.Request, status int, id string) {
	g, err := rt.scim.GetSCIMGroup(r.Context(), id)
	if err != nil {
		rt.scimStoreError(w, "load group", err)
		return
	}
	writeSCIM(w, status, rt.scimGroupResource(r, g))
}

func (rt *Router) handleSCIMListGroups(w http.ResponseWriter, r *http.Request, _ store.SCIMToken) {
	f, err := scim.ParseFilter(r.URL.Query().Get("filter"), store.SCIMGroupFilterDisplayName, store.SCIMGroupFilterExternalID, store.SCIMGroupFilterID)
	if err != nil {
		writeSCIMError(w, http.StatusBadRequest, scimTypeInvalidFilter, "supported: displayName, externalId or id with eq")
		return
	}
	attr, value := "", ""
	if f != nil {
		attr, value = f.Attribute, f.Value
	}
	offset, limit, start := scimPage(r)
	groups, total, err := rt.scim.ListSCIMGroups(r.Context(), attr, value, offset, limit)
	if err != nil {
		rt.scimStoreError(w, "list groups", err)
		return
	}
	out := scim.GroupList{ListHeader: scim.NewListHeader(len(groups), total, start), Resources: []scim.Group{}}
	for _, g := range groups {
		out.Resources = append(out.Resources, rt.scimGroupResource(r, g))
	}
	writeSCIM(w, http.StatusOK, out)
}

func (rt *Router) handleSCIMGetGroup(w http.ResponseWriter, r *http.Request, _ store.SCIMToken) {
	rt.writeSCIMGroup(w, r, http.StatusOK, r.PathValue("id"))
}

func memberIDs(refs []scim.Ref) []string {
	ids := make([]string, 0, len(refs))
	for _, m := range refs {
		if v := strings.TrimSpace(m.Value); v != "" {
			ids = append(ids, v)
		}
	}
	return ids
}

// saveSCIMGroup writes the group and its members in one transaction, then
// reapplies role mappings to every user whose membership changed.
func (rt *Router) saveSCIMGroup(w http.ResponseWriter, r *http.Request, tok store.SCIMToken, g store.SCIMGroup, create bool, changes []store.MemberChange) {
	if strings.TrimSpace(g.DisplayName) == "" {
		writeSCIMError(w, http.StatusBadRequest, scimTypeInvalidValue, "displayName is required")
		return
	}
	touched, err := rt.scim.SaveSCIMGroup(r.Context(), g, create, changes, time.Now())
	if err != nil {
		rt.scimStoreError(w, "save group", err)
		return
	}
	action, status := auditActionSCIMGroupUpdated, http.StatusOK
	if create {
		action, status = auditActionSCIMGroupCreated, http.StatusCreated
	}
	rt.auditSCIM(r, tok, action, "/Groups/"+g.ID, status)
	if err := rt.syncDirectoryRoles(r, tok, touched); err != nil {
		rt.scimStoreError(w, "sync roles", err)
		return
	}
	rt.writeSCIMGroup(w, r, status, g.ID)
}

func (rt *Router) handleSCIMCreateGroup(w http.ResponseWriter, r *http.Request, tok store.SCIMToken) {
	var in scim.Group
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeSCIMError(w, http.StatusBadRequest, scimTypeInvalidValue, "invalid body")
		return
	}
	id, err := randomOpaqueID("grp_")
	if err != nil {
		rt.scimStoreError(w, "create group", err)
		return
	}
	g := store.SCIMGroup{ID: id, DisplayName: strings.TrimSpace(in.DisplayName), ExternalID: in.ExternalID}
	rt.saveSCIMGroup(w, r, tok, g, true, []store.MemberChange{{Add: memberIDs(in.Members)}})
}

func (rt *Router) handleSCIMReplaceGroup(w http.ResponseWriter, r *http.Request, tok store.SCIMToken) {
	cur, err := rt.scim.GetSCIMGroup(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.scimStoreError(w, "replace group", err)
		return
	}
	var in scim.Group
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeSCIMError(w, http.StatusBadRequest, scimTypeInvalidValue, "invalid body")
		return
	}
	cur.DisplayName, cur.ExternalID = strings.TrimSpace(in.DisplayName), in.ExternalID
	rt.saveSCIMGroup(w, r, tok, cur, false, []store.MemberChange{{Replace: true, Add: memberIDs(in.Members)}})
}

func (rt *Router) handleSCIMPatchGroup(w http.ResponseWriter, r *http.Request, tok store.SCIMToken) {
	cur, err := rt.scim.GetSCIMGroup(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.scimStoreError(w, "patch group", err)
		return
	}
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeSCIMError(w, http.StatusBadRequest, scimTypeInvalidValue, "invalid body")
		return
	}
	change, err := scim.ParseGroupPatch(raw)
	if err != nil {
		writeSCIMError(w, http.StatusBadRequest, scimTypeInvalidValue, err.Error())
		return
	}
	if change.DisplayName != nil {
		cur.DisplayName = *change.DisplayName
	}
	if change.ExternalID != nil {
		cur.ExternalID = *change.ExternalID
	}
	changes := make([]store.MemberChange, 0, len(change.Members))
	for _, op := range change.Members {
		switch op.Kind {
		case scim.MembersAdd:
			changes = append(changes, store.MemberChange{Add: op.IDs})
		case scim.MembersRemove:
			changes = append(changes, store.MemberChange{Remove: op.IDs})
		case scim.MembersReplace:
			changes = append(changes, store.MemberChange{Replace: true, Add: op.IDs})
		}
	}
	rt.saveSCIMGroup(w, r, tok, cur, false, changes)
}

func (rt *Router) handleSCIMDeleteGroup(w http.ResponseWriter, r *http.Request, tok store.SCIMToken) {
	id := r.PathValue("id")
	members, err := rt.scim.DeleteSCIMGroup(r.Context(), id)
	if err != nil {
		rt.scimStoreError(w, "delete group", err)
		return
	}
	rt.auditSCIM(r, tok, auditActionSCIMGroupDeleted, "/Groups/"+id, http.StatusNoContent)
	if err := rt.syncDirectoryRoles(r, tok, members); err != nil {
		rt.scimStoreError(w, "sync roles", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
