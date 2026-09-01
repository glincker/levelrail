package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// PolicyStore is the store surface the IAM policy handlers need,
// defined here next to the handlers that use it, the same
// "single-file feature" shape OnboardingStore/AuditStore already use
// rather than every store interface living in store_interfaces.go.
type PolicyStore interface {
	SavePolicy(ctx context.Context, p store.Policy) error
	UpdatePolicy(ctx context.Context, id, name, description, document string) error
	GetPolicy(ctx context.Context, id string) (*store.Policy, error)
	ListPolicies(ctx context.Context) ([]store.Policy, error)
	DeletePolicy(ctx context.Context, id string) error
	AttachPolicy(ctx context.Context, id, policyID, principalType, principalID string) error
	DetachPolicy(ctx context.Context, policyID, principalType, principalID string) error
	ListPoliciesForPrincipal(ctx context.Context, principalType, principalID string) ([]store.Policy, error)
	ListAttachmentsForPolicy(ctx context.Context, policyID string) ([]store.PolicyAttachment, error)
}

var validPrincipalTypes = []string{store.PrincipalTypeUser, store.PrincipalTypeToken}

type policyResource struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Document    json.RawMessage `json:"document"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

func toPolicyResource(p store.Policy) policyResource {
	return policyResource{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Document:    json.RawMessage(p.Document),
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

type policyAttachmentResource struct {
	ID            string    `json:"id"`
	PolicyID      string    `json:"policy_id"`
	PrincipalType string    `json:"principal_type"`
	PrincipalID   string    `json:"principal_id"`
	CreatedAt     time.Time `json:"created_at"`
}

func toPolicyAttachmentResource(a store.PolicyAttachment) policyAttachmentResource {
	return policyAttachmentResource{
		ID:            a.ID,
		PolicyID:      a.PolicyID,
		PrincipalType: a.PrincipalType,
		PrincipalID:   a.PrincipalID,
		CreatedAt:     a.CreatedAt,
	}
}

type policyRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Document    json.RawMessage `json:"document"`
}

// handleCreatePolicy handles POST /api/v1/iam/policies. AbilityRoot:
// minting a policy is at least as consequential as minting a token with
// arbitrary abilities (handleCreateUser's own tier), since a Deny
// statement can silently override abilities a caller already holds.
func (rt *Router) handleCreatePolicy(w http.ResponseWriter, r *http.Request) {
	var req policyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if _, err := ParseDocument(string(req.Document)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := store.NewPolicyID()
	if err != nil {
		rt.logger.Error("api: create policy: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	now := time.Now()
	rec := store.Policy{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Document:    string(req.Document),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := rt.policies.SavePolicy(r.Context(), rec); err != nil {
		if errors.Is(err, store.ErrPolicyNameExists) {
			writeError(w, http.StatusConflict, "a policy with this name already exists")
			return
		}
		rt.logger.Error("api: create policy: save failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, toPolicyResource(rec))
}

// handleListPolicies handles GET /api/v1/iam/policies.
func (rt *Router) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	recs, err := rt.policies.ListPolicies(r.Context())
	if err != nil {
		rt.logger.Error("api: list policies failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]policyResource, 0, len(recs))
	for _, p := range recs {
		out = append(out, toPolicyResource(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetPolicy handles GET /api/v1/iam/policies/{id}.
func (rt *Router) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rec, err := rt.policies.GetPolicy(r.Context(), id)
	if errors.Is(err, store.ErrPolicyNotFound) {
		writeError(w, http.StatusNotFound, "policy not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get policy failed", slog.String("error", err.Error()), slog.String("policy_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toPolicyResource(*rec))
}

// handleUpdatePolicy handles PUT /api/v1/iam/policies/{id}.
func (rt *Router) handleUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req policyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if _, err := ParseDocument(string(req.Document)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	err := rt.policies.UpdatePolicy(r.Context(), id, req.Name, req.Description, string(req.Document))
	if errors.Is(err, store.ErrPolicyNotFound) {
		writeError(w, http.StatusNotFound, "policy not found")
		return
	}
	if errors.Is(err, store.ErrPolicyNameExists) {
		writeError(w, http.StatusConflict, "a policy with this name already exists")
		return
	}
	if err != nil {
		rt.logger.Error("api: update policy failed", slog.String("error", err.Error()), slog.String("policy_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rec, err := rt.policies.GetPolicy(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: reload policy after update failed", slog.String("error", err.Error()), slog.String("policy_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toPolicyResource(*rec))
}

// handleDeletePolicy handles DELETE /api/v1/iam/policies/{id}.
// Idempotent at the store layer (DeletePolicy), so this never 404s.
func (rt *Router) handleDeletePolicy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := rt.policies.DeletePolicy(r.Context(), id); err != nil {
		rt.logger.Error("api: delete policy failed", slog.String("error", err.Error()), slog.String("policy_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type attachPolicyRequest struct {
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
}

// handleAttachPolicy handles POST /api/v1/iam/policies/{id}/attachments.
// Idempotent at the store layer (AttachPolicy).
func (rt *Router) handleAttachPolicy(w http.ResponseWriter, r *http.Request) {
	policyID := r.PathValue("id")
	var req attachPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !slices.Contains(validPrincipalTypes, req.PrincipalType) {
		writeError(w, http.StatusBadRequest, "principal_type must be \"user\" or \"token\"")
		return
	}
	if req.PrincipalID == "" {
		writeError(w, http.StatusBadRequest, "principal_id is required")
		return
	}
	if _, err := rt.policies.GetPolicy(r.Context(), policyID); errors.Is(err, store.ErrPolicyNotFound) {
		writeError(w, http.StatusNotFound, "policy not found")
		return
	} else if err != nil {
		rt.logger.Error("api: attach policy: lookup failed", slog.String("error", err.Error()), slog.String("policy_id", policyID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	id, err := store.NewPolicyAttachmentID()
	if err != nil {
		rt.logger.Error("api: attach policy: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := rt.policies.AttachPolicy(r.Context(), id, policyID, req.PrincipalType, req.PrincipalID); err != nil {
		rt.logger.Error("api: attach policy failed", slog.String("error", err.Error()), slog.String("policy_id", policyID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDetachPolicy handles DELETE
// /api/v1/iam/policies/{id}/attachments/{principal_type}/{principal_id}.
// Idempotent at the store layer (DetachPolicy).
func (rt *Router) handleDetachPolicy(w http.ResponseWriter, r *http.Request) {
	policyID := r.PathValue("id")
	principalType := r.PathValue("principal_type")
	principalID := r.PathValue("principal_id")
	if err := rt.policies.DetachPolicy(r.Context(), policyID, principalType, principalID); err != nil {
		rt.logger.Error("api: detach policy failed", slog.String("error", err.Error()), slog.String("policy_id", policyID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListPolicyAttachments handles GET
// /api/v1/iam/policies/{id}/attachments: which principals a policy
// currently applies to.
func (rt *Router) handleListPolicyAttachments(w http.ResponseWriter, r *http.Request) {
	policyID := r.PathValue("id")
	recs, err := rt.policies.ListAttachmentsForPolicy(r.Context(), policyID)
	if err != nil {
		rt.logger.Error("api: list policy attachments failed", slog.String("error", err.Error()), slog.String("policy_id", policyID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]policyAttachmentResource, 0, len(recs))
	for _, a := range recs {
		out = append(out, toPolicyAttachmentResource(a))
	}
	writeJSON(w, http.StatusOK, out)
}
