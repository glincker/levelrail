package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/ai"
	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

// EnvAITurnTokenTTL overrides how long one assistant turn's token lives.
const EnvAITurnTokenTTL = "APP_AI_TURN_TOKEN_TTL" //nolint:gosec // an env var name, not a credential

const defaultAITurnTokenTTL = 15 * time.Minute

func aiTurnTokenTTL() time.Duration {
	if v, ok := os.LookupEnv(EnvAITurnTokenTTL); ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultAITurnTokenTTL
}

// aiTurnState is what the API learns about one live turn token's requests.
type aiTurnState struct {
	onBehalfOf string
	mu         sync.Mutex
	abilities  []string
}

func (s *aiTurnState) note(ability string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.Contains(s.abilities, ability) {
		s.abilities = append(s.abilities, ability)
	}
}

func (s *aiTurnState) drain() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.abilities
	s.abilities = nil
	return out
}

type aiTurnRegistry struct {
	mu    sync.Mutex
	turns map[string]*aiTurnState
}

func (g *aiTurnRegistry) add(tokenID string, s *aiTurnState) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.turns == nil {
		g.turns = map[string]*aiTurnState{}
	}
	g.turns[tokenID] = s
}

func (g *aiTurnRegistry) remove(tokenID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.turns, tokenID)
}

func (g *aiTurnRegistry) get(tokenID string) *aiTurnState {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.turns[tokenID]
}

type onBehalfOfKey struct{}

func withOnBehalfOf(ctx context.Context, principal string) context.Context {
	return context.WithValue(ctx, onBehalfOfKey{}, principal)
}

func onBehalfOfFrom(ctx context.Context) string {
	s, _ := ctx.Value(onBehalfOfKey{}).(string)
	return s
}

// noteAITurnRequest records the ability a turn token's request needs and
// tags the request with the human the turn acts for, for the audit row.
func (rt *Router) noteAITurnRequest(r *http.Request, rec *store.APIToken, required string) *http.Request {
	turn := rt.aiTurns.get(rec.ID)
	if turn == nil {
		return r
	}
	turn.note(required)
	return r.WithContext(withOnBehalfOf(r.Context(), turn.onBehalfOf))
}

// aiTurnSender is the human principal a chat turn runs as.
type aiTurnSender struct {
	principalType, principalID string
	ownerUserID                string
	abilities                  []string
	display                    string
	scope                      *store.APITokenScope
}

func (s aiTurnSender) ref() string { return s.principalType + ":" + s.principalID }

func (rt *Router) aiTurnSenderOf(r *http.Request) (aiTurnSender, error) {
	ctx := r.Context()
	ptype, pid, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		return aiTurnSender{}, err
	}
	s := aiTurnSender{principalType: ptype, principalID: pid, abilities: abilities}
	if ptype == store.PrincipalTypeUser {
		user, err := rt.auth.GetUserByID(ctx, pid)
		if err != nil {
			return aiTurnSender{}, fmt.Errorf("api: ai turn: load sender: %w", err)
		}
		s.ownerUserID, s.display = pid, user.Email
		return s, nil
	}
	rec, err := rt.tokens.GetAPITokenByID(ctx, pid)
	if err != nil {
		return aiTurnSender{}, fmt.Errorf("api: ai turn: load sender token: %w", err)
	}
	s.ownerUserID, s.display = rec.OwnerUserID, rec.Name
	if db := rt.teamDB(); db != nil {
		if s.scope, err = db.GetAPITokenScope(ctx, pid); err != nil {
			return aiTurnSender{}, fmt.Errorf("api: ai turn: load sender scope: %w", err)
		}
	}
	return s, nil
}

// beginAITurn mints the token one chat turn's tools run with: owned by the
// sender, carrying only the sender's abilities, IAM policies and team scope,
// labeled as the assistant so the AI control gate applies, and revoked by end.
func (rt *Router) beginAITurn(r *http.Request) (cred ai.TurnCredential, end func(), err error) {
	sender, err := rt.aiTurnSenderOf(r)
	if err != nil {
		return ai.TurnCredential{}, nil, err
	}
	ctx := context.WithoutCancel(r.Context())
	expires := time.Now().Add(aiTurnTokenTTL())
	raw, tokenID, err := rt.mintAITurnToken(ctx, sender, expires)
	if err != nil {
		return ai.TurnCredential{}, nil, err
	}
	var attached []string
	end = func() {
		if rerr := rt.libraryRevokeToken(ctx, tokenID); rerr != nil {
			rt.logger.Error("api: ai turn: revoke token failed", slog.String("token_id", tokenID), slog.String("error", rerr.Error()))
		}
		rt.aiTurns.remove(tokenID)
		for _, policyID := range attached {
			if derr := rt.policies.DetachPolicy(ctx, policyID, store.PrincipalTypeToken, tokenID); derr != nil {
				rt.logger.Warn("api: ai turn: detach policy failed", slog.String("token_id", tokenID), slog.String("error", derr.Error()))
			}
		}
	}
	if attached, err = rt.copyAITurnRestrictions(ctx, sender, tokenID); err != nil {
		end()
		return ai.TurnCredential{}, nil, err
	}
	state := &aiTurnState{onBehalfOf: sender.ref()}
	rt.aiTurns.add(tokenID, state)
	return ai.TurnCredential{Token: raw, ActingAs: sender.display, AbilitiesUsed: state.drain}, end, nil
}

func (rt *Router) mintAITurnToken(ctx context.Context, s aiTurnSender, expires time.Time) (raw, tokenID string, err error) {
	agent := agentIdentity{Name: AIAssistantAgentName, Description: "one assistant chat turn"}
	if s.ownerUserID == "" {
		raw, rec, err := MintAgentAPIToken(ctx, rt.tokens, AIAssistantTokenName, s.abilities, &expires, agent, "")
		return raw, rec.ID, err
	}
	legacyID, err := randomTokenID()
	if err != nil {
		return "", "", err
	}
	raw, lib, err := rt.authLib.tokens.MintToken(ctx, authengine.MintInput{
		OwnerLegacyID: s.ownerUserID, Name: AIAssistantTokenName, Abilities: s.abilities,
		ExpiresAt: &expires, AgentName: agent.Name,
	})
	if err != nil {
		return "", "", fmt.Errorf("api: ai turn: mint: %w", err)
	}
	mirror := store.APIToken{
		ID: legacyID, Name: lib.Name, TokenHash: lib.HashHex, Abilities: s.abilities, CreatedAt: lib.CreatedAt,
		ExpiresAt: &expires, AgentName: agent.Name, AgentDescription: agent.Description, OwnerUserID: s.ownerUserID,
	}
	err = rt.tokens.SaveAPIToken(ctx, mirror)
	if err == nil {
		err = rt.authLib.tokens.LinkToken(ctx, legacyID, lib.EngineID)
	}
	if err != nil {
		rerr := rt.authLib.tokens.RevokeToken(ctx, lib.EngineID)
		if lerr := rt.tokens.RevokeAPIToken(ctx, legacyID); lerr != nil && !errors.Is(lerr, store.ErrAPITokenNotFound) {
			rerr = errors.Join(rerr, lerr)
		}
		return "", "", errors.Join(fmt.Errorf("api: ai turn: mirror: %w", err), rerr)
	}
	return raw, legacyID, nil
}

// copyAITurnRestrictions attaches the sender's IAM policies and team scope
// to the turn token, so a Deny on the sender also binds the assistant.
func (rt *Router) copyAITurnRestrictions(ctx context.Context, s aiTurnSender, tokenID string) ([]string, error) {
	policies, err := rt.policies.ListPoliciesForPrincipal(ctx, s.principalType, s.principalID)
	if err != nil {
		return nil, fmt.Errorf("api: ai turn: list sender policies: %w", err)
	}
	var attached []string
	for _, p := range policies {
		id, err := store.NewPolicyAttachmentID()
		if err == nil {
			err = rt.policies.AttachPolicy(ctx, id, p.ID, store.PrincipalTypeToken, tokenID)
		}
		if err != nil {
			return attached, fmt.Errorf("api: ai turn: copy policy %s: %w", p.ID, err)
		}
		attached = append(attached, p.ID)
	}
	if s.scope == nil {
		return attached, nil
	}
	db := rt.teamAdmin()
	if db == nil {
		return attached, errors.New("api: ai turn: store lacks token scopes")
	}
	scope := *s.scope
	scope.TokenID = tokenID
	if err := db.SetAPITokenScope(ctx, scope); err != nil {
		return attached, fmt.Errorf("api: ai turn: copy scope: %w", err)
	}
	return attached, nil
}

// startAITurn is beginAITurn for a handler that has not written a response yet.
func (rt *Router) startAITurn(w http.ResponseWriter, r *http.Request) (*http.Request, func(), bool) {
	cred, end, err := rt.beginAITurn(r)
	if err != nil {
		rt.internalError(w, "api: ai turn: start failed", err)
		return nil, nil, false
	}
	return r.WithContext(ai.WithTurnCredential(r.Context(), cred)), end, true
}

// spentAITurnToken hides a finished turn's token from token lists: every
// chat message mints one, and a dead one carries no information.
func spentAITurnToken(t store.APIToken, now time.Time) bool {
	if t.Name != AIAssistantTokenName || t.AgentName != AIAssistantAgentName || t.OwnerUserID == "" {
		return false
	}
	return t.RevokedAt != nil || (t.ExpiresAt != nil && now.After(*t.ExpiresAt))
}
