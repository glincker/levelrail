package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	changeAdded   = "added"
	changeRemoved = "removed"
)

// policyVersionStore is the optional history surface; *store.DB satisfies it.
type policyVersionStore interface {
	SavePolicyVersion(ctx context.Context, policyID, name, description, document, actor string) error
	ListPolicyVersions(ctx context.Context, policyID string) ([]store.PolicyVersion, error)
}

type statementChange struct {
	Change    string    `json:"change"`
	Statement Statement `json:"statement"`
}

type policyVersionResource struct {
	Version     int               `json:"version"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Document    json.RawMessage   `json:"document"`
	Actor       string            `json:"actor"`
	CreatedAt   time.Time         `json:"created_at"`
	Changes     []statementChange `json:"changes"`
}

func statementKey(s Statement) string {
	a, r := slices.Clone(s.Action), slices.Clone(s.Resource)
	slices.Sort(a)
	slices.Sort(r)
	return string(s.Effect) + "|" + strings.Join(a, ",") + "|" + strings.Join(r, ",")
}

// diffDocuments lists statements added to and removed from older to newer.
// Unparseable documents diff as empty.
func diffDocuments(older, newer string) []statementChange {
	changes := []statementChange{}
	od, _ := ParseDocument(older)
	nd, _ := ParseDocument(newer)
	var oldS, newS []Statement
	if od != nil {
		oldS = od.Statement
	}
	if nd != nil {
		newS = nd.Statement
	}
	has := func(list []Statement, s Statement) bool {
		return slices.ContainsFunc(list, func(o Statement) bool { return statementKey(o) == statementKey(s) })
	}
	for _, s := range oldS {
		if !has(newS, s) {
			changes = append(changes, statementChange{Change: changeRemoved, Statement: s})
		}
	}
	for _, s := range newS {
		if !has(oldS, s) {
			changes = append(changes, statementChange{Change: changeAdded, Statement: s})
		}
	}
	return changes
}

// actorForRequest names the caller of a mutating request, user:ID or token:ID.
func (rt *Router) actorForRequest(r *http.Request) string {
	ptype, pid, _, err := rt.callerPrincipal(r)
	if err != nil {
		return ""
	}
	return ptype + ":" + pid
}

// recordPolicyVersion appends history. A failure is logged and never fails the
// save, since the live document is already stored.
func (rt *Router) recordPolicyVersion(r *http.Request, p store.Policy) {
	vs, ok := rt.policies.(policyVersionStore)
	if !ok {
		return
	}
	if err := vs.SavePolicyVersion(r.Context(), p.ID, p.Name, p.Description, p.Document, rt.actorForRequest(r)); err != nil {
		rt.logger.Warn("api: record policy version failed", slog.String("error", err.Error()), slog.String("policy_id", p.ID))
	}
}

// handleListPolicyVersions handles GET /api/v1/iam/policies/{id}/versions,
// newest first, each with its statement changes against the version before.
func (rt *Router) handleListPolicyVersions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	vs, ok := rt.policies.(policyVersionStore)
	if !ok {
		writeJSON(w, http.StatusOK, []policyVersionResource{})
		return
	}
	list, err := vs.ListPolicyVersions(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: list policy versions failed", err, slog.String("policy_id", id))
		return
	}
	out := make([]policyVersionResource, 0, len(list))
	for i, v := range list {
		res := policyVersionResource{Version: v.Version, Name: v.Name, Description: v.Description, Document: json.RawMessage(v.Document), Actor: v.Actor, CreatedAt: v.CreatedAt, Changes: []statementChange{}}
		if i+1 < len(list) {
			res.Changes = diffDocuments(list[i+1].Document, v.Document)
		} else {
			res.Changes = diffDocuments("", v.Document)
		}
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, out)
}
