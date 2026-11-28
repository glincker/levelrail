package api

import (
	"context"

	"github.com/glincker/levelrail/internal/alerting"
	"github.com/glincker/levelrail/internal/store"
)

// PushRecipientStore is the store surface PushRecipients needs; *store.DB satisfies it.
type PushRecipientStore interface {
	GetUserByID(ctx context.Context, id string) (*store.User, error)
	UserTeamRoles(ctx context.Context, userID string) (map[string]string, error)
	OwnerOfApp(ctx context.Context, name string) (*store.ResourceOwner, error)
}

// PushRecipients decides who receives a browser push, using the same team
// scope and ability rules that gate the dashboard and API.
type PushRecipients struct{ db PushRecipientStore }

// NewPushRecipients builds the recipient policy for the web push sender.
func NewPushRecipients(db PushRecipientStore) *PushRecipients { return &PushRecipients{db: db} }

// AllowPush reports whether userID may receive a push meant for audience.
func (p *PushRecipients) AllowPush(ctx context.Context, userID string, audience alerting.PushAudience) bool {
	user, err := p.db.GetUserByID(ctx, userID)
	if err != nil {
		return false
	}
	scope, err := teamScopeOfUser(ctx, p.db, user)
	if err != nil || (scope != nil && !scope.instance) {
		return false
	}
	if audience.UserID != "" && audience.UserID != userID {
		return false
	}
	if audience.RootOnly && scope != nil {
		return false
	}
	var owner *store.ResourceOwner
	if audience.AppName != "" {
		if owner, err = p.db.OwnerOfApp(ctx, audience.AppName); err != nil {
			return false
		}
	}
	if owner == nil {
		if audience.UserID != "" {
			return true
		}
		return scope == nil || store.TeamRoleAtLeast(scope.roleOfTeam(store.DefaultTeamID), store.TeamRoleAdmin)
	}
	if audience.ProjectID != "" && owner.ProjectID != audience.ProjectID {
		return false
	}
	role := scope.roleFor(owner)
	if role == "" || !hasAbility(capAbilities(user.Abilities, owner.TeamID, role), AbilityRead) {
		return false
	}
	return !audience.AdminOnly || scope == nil || store.TeamRoleAtLeast(role, store.TeamRoleAdmin)
}
