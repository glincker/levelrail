package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// nodeJoinTokenTTL is how long a minted join token stays redeemable
// before an operator has to mint a new one. Not (yet) an env-configurable
// threshold: this is a one-shot enrollment window, not an ongoing
// operational setting like APP_SESSION_TTL, so CLAUDE.md 7's "no
// hardcoded thresholds" concern doesn't apply the same way here. Revisit
// if real usage shows this needs to be adjustable.
const nodeJoinTokenTTL = 15 * time.Minute

// NodeStore is the store surface the node-management handlers need.
// *store.DB satisfies this structurally, the same narrow
// consumer-defined interface convention every other Store sub-interface
// in this package already follows.
type NodeStore interface {
	ListNodes(ctx context.Context) ([]store.Node, error)
	GetNode(ctx context.Context, id string) (*store.Node, error)
	DeleteNode(ctx context.Context, id string) error
	SaveNodeJoinToken(ctx context.Context, t store.NodeJoinToken) error
}

// nodeResource is the wire shape for a node.
type nodeResource struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Address         string     `json:"address,omitempty"`
	Status          string     `json:"status"`
	CertFingerprint string     `json:"cert_fingerprint,omitempty"`
	JoinedAt        *time.Time `json:"joined_at,omitempty"`
	LastSeenAt      *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

func toNodeResource(n store.Node) nodeResource {
	return nodeResource{
		ID:              n.ID,
		Name:            n.Name,
		Address:         n.Address,
		Status:          string(n.Status),
		CertFingerprint: n.CertFingerprint,
		JoinedAt:        n.JoinedAt,
		LastSeenAt:      n.LastSeenAt,
		CreatedAt:       n.CreatedAt,
	}
}

// handleListNodes handles GET /api/v1/nodes (TASKS.md 3.1).
func (rt *Router) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := rt.nodes.ListNodes(r.Context())
	if err != nil {
		rt.logger.Error("api: list nodes failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]nodeResource, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, toNodeResource(n))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetNode handles GET /api/v1/nodes/{id}.
func (rt *Router) handleGetNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, err := rt.nodes.GetNode(r.Context(), id)
	if errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get node failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toNodeResource(*n))
}

// handleDeleteNode handles DELETE /api/v1/nodes/{id}. Idempotent at the
// store layer (store.DeleteNode), matching handleDeleteAlertRule's own
// idempotent-delete shape.
//
// No placement guard yet (TASKS.md 3.3 hasn't landed: no service can be
// assigned to a node at all as of this pass, store.DeleteNode's own doc
// comment covers why that makes the guard currently vacuous, not
// unwritten), and no drain (TASKS.md 3.7). Deleting an online,
// in-use node today just deletes its registry row; the node itself, and
// whatever real gRPC connection TASKS.md 3.2 eventually holds for it, is
// unaffected by this call, since neither exists yet either.
func (rt *Router) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := rt.nodes.DeleteNode(r.Context(), id); err != nil {
		rt.logger.Error("api: delete node failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// createNodeJoinTokenResponse is a join token's one and only appearance
// in plaintext, the same "shown once, never recoverable again" shape
// createTokenResponse (tokens.go) already established for API tokens.
type createNodeJoinTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// handleCreateNodeJoinToken handles POST /api/v1/nodes/join-tokens
// (TASKS.md 3.1): mints a one-time token an operator pastes into a new
// node's enrollment command (TASKS.md 3.2, `cmd/levelrail-agent`, not
// built yet, is what will eventually exchange this token for a client
// certificate). Nothing in this codebase redeems a token yet; this
// handler only mints and persists the hash.
func (rt *Router) handleCreateNodeJoinToken(w http.ResponseWriter, r *http.Request) {
	plaintext, err := randomToken()
	if err != nil {
		rt.logger.Error("api: create node join token: generate token failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	id, err := randomNodeJoinTokenID()
	if err != nil {
		rt.logger.Error("api: create node join token: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	now := time.Now()
	rec := store.NodeJoinToken{
		ID:        id,
		TokenHash: hashToken(plaintext),
		CreatedAt: now,
		ExpiresAt: now.Add(nodeJoinTokenTTL),
	}
	if err := rt.nodes.SaveNodeJoinToken(r.Context(), rec); err != nil {
		rt.logger.Error("api: create node join token: save failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, createNodeJoinTokenResponse{Token: plaintext, ExpiresAt: rec.ExpiresAt})
}

// randomNodeJoinTokenID generates a short, URL-safe, non-secret
// identifier for a join-token row, the exact shape randomTokenID
// (tokens.go) already establishes for API tokens, duplicated rather than
// shared: the two ID spaces are for genuinely different resources and
// nothing depends on them being interchangeable.
func randomNodeJoinTokenID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate node join token id: %w", err)
	}
	return "njt_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
