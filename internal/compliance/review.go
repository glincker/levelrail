package compliance

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// UserRemover removes an account together with its live sessions. The API
// layer supplies it so every guard of the normal delete path still applies.
type UserRemover interface {
	RemoveUser(ctx context.Context, userID, actorID string) error
}

func newID(prefix string) (string, error) {
	b := make([]byte, 9)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("compliance: generate id: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}

func stamp(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// Snapshot lists every user, team membership, live API token and
// environment grant with its last-active time.
func Snapshot(ctx context.Context, db *store.DB) ([]store.AccessReviewItem, error) {
	users, err := db.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("compliance: list users: %w", err)
	}
	active := map[string]string{}
	var items []store.AccessReviewItem
	add := func(it store.AccessReviewItem) error {
		id, err := newID("ari_")
		if err != nil {
			return err
		}
		it.ID = id
		items = append(items, it)
		return nil
	}
	for _, u := range users {
		seen := stamp(u.LastLoginAt)
		active[u.ID] = seen
		abilities := slices.Clone(u.Abilities)
		slices.Sort(abilities)
		if err := add(store.AccessReviewItem{Kind: store.AccessSubjectUser, SubjectID: u.ID, Label: u.Email,
			Role: strings.Join(abilities, ","), Detail: u.DisplayName, LastActiveAt: seen}); err != nil {
			return nil, err
		}
	}
	orgs, err := db.ListOrganizations(ctx)
	if err != nil {
		return nil, fmt.Errorf("compliance: list teams: %w", err)
	}
	for _, o := range orgs {
		members, err := db.ListTeamMembers(ctx, o.ID)
		if err != nil {
			return nil, fmt.Errorf("compliance: list team members: %w", err)
		}
		for _, m := range members {
			if err := add(store.AccessReviewItem{Kind: store.AccessSubjectTeamMember, SubjectID: o.ID + "/" + m.UserID,
				Label: m.Email + " in " + o.Name, Role: m.Role, Detail: o.Name, LastActiveAt: active[m.UserID]}); err != nil {
				return nil, err
			}
		}
	}
	tokens, err := db.ListAPITokens(ctx)
	if err != nil {
		return nil, fmt.Errorf("compliance: list api tokens: %w", err)
	}
	for _, t := range tokens {
		if t.RevokedAt != nil {
			continue
		}
		seen := stamp(t.LastUsedAt)
		detail := "owner " + t.OwnerUserID
		if t.AgentName != "" {
			detail = "agent " + t.AgentName
		}
		abilities := slices.Clone(t.Abilities)
		slices.Sort(abilities)
		if err := add(store.AccessReviewItem{Kind: store.AccessSubjectAPIToken, SubjectID: t.ID, Label: t.Name,
			Role: strings.Join(abilities, ","), Detail: detail, LastActiveAt: seen}); err != nil {
			return nil, err
		}
	}
	for _, u := range users {
		envs, err := db.ListUserEnvironmentGrants(ctx, u.ID)
		if err != nil {
			return nil, fmt.Errorf("compliance: list environment grants: %w", err)
		}
		for _, env := range envs {
			if err := add(store.AccessReviewItem{Kind: store.AccessSubjectEnvGrant, SubjectID: u.ID + "/" + env,
				Label: u.Email + " on environment " + env, Role: "environment grant", Detail: env, LastActiveAt: active[u.ID]}); err != nil {
				return nil, err
			}
		}
	}
	return items, nil
}

// applyRevocation removes one subject's access through the same store
// primitives the normal management screens use. An already-gone subject
// counts as applied so a retry after a half-finished completion is safe.
func (s *Service) applyRevocation(ctx context.Context, it store.AccessReviewItem, actorID string) error {
	switch it.Kind {
	case store.AccessSubjectUser:
		if s.users == nil {
			return errors.New("user removal is not available on this control plane")
		}
		if err := s.users.RemoveUser(ctx, it.SubjectID, actorID); err != nil && !errors.Is(err, store.ErrUserNotFound) {
			return err
		}
	case store.AccessSubjectTeamMember:
		team, user, ok := strings.Cut(it.SubjectID, "/")
		if !ok {
			return fmt.Errorf("malformed team member subject %q", it.SubjectID)
		}
		if err := s.db.RemoveTeamMember(ctx, team, user); err != nil && !errors.Is(err, store.ErrTeamMemberNotFound) {
			return err
		}
	case store.AccessSubjectAPIToken:
		if err := s.db.RevokeAPIToken(ctx, it.SubjectID); err != nil && !errors.Is(err, store.ErrAPITokenNotFound) {
			return err
		}
	case store.AccessSubjectEnvGrant:
		user, env, ok := strings.Cut(it.SubjectID, "/")
		if !ok {
			return fmt.Errorf("malformed grant subject %q", it.SubjectID)
		}
		grants, err := s.db.ListUserEnvironmentGrants(ctx, user)
		if err != nil {
			return err
		}
		kept := slices.DeleteFunc(slices.Clone(grants), func(g string) bool { return g == env })
		if len(kept) != len(grants) {
			return s.db.SetUserEnvironmentGrants(ctx, user, kept)
		}
	default:
		return fmt.Errorf("unknown subject kind %q", it.Kind)
	}
	return nil
}
