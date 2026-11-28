package alerting

import (
	"context"
	"strings"

	"github.com/glincker/levelrail/internal/store"
)

// PushAudience says which users may receive a browser push. The zero value
// is the narrowest: instance wide notices go to admins only.
type PushAudience struct {
	// AppName limits the audience to users who can read that app.
	AppName string
	// AdminOnly narrows an app audience to admin and owner roles.
	AdminOnly bool
	// RootOnly limits the audience to root accounts.
	RootOnly bool
	// UserID limits the audience to one account.
	UserID string
	// ProjectID is the project of the channel carrying the notice; empty for any.
	ProjectID string
}

type pushAudienceKey struct{}

// WithPushAudience attaches the audience a notice or event is meant for.
func WithPushAudience(ctx context.Context, a PushAudience) context.Context {
	return context.WithValue(ctx, pushAudienceKey{}, a)
}

// PushAudienceFrom returns the audience set on ctx, or the zero value.
func PushAudienceFrom(ctx context.Context) PushAudience {
	a, _ := ctx.Value(pushAudienceKey{}).(PushAudience)
	return a
}

// pushAudienceForResource maps a rule resource to its audience; only app
// resources have a narrower one.
func pushAudienceForResource(resourceID string) PushAudience {
	if app, ok := strings.CutPrefix(resourceID, "service:"); ok && app != "" {
		return PushAudience{AppName: app}
	}
	return PushAudience{}
}

// domainOwnerResolver is the optional store surface that names the app holding a domain.
// It resolves apps only, so a static site that shares an app's name never maps to that app's team.
type domainOwnerResolver interface {
	DomainAppOwnerOf(ctx context.Context, domain string) (owner string, found bool, err error)
}

// resolvePushAudience maps a domain rule to its owning app's team admins plus root.
// Nodes and volumes have no team owner in the store, so they keep the admin-only zero audience.
func resolvePushAudience(ctx context.Context, src any, resourceID string) PushAudience {
	a := pushAudienceForResource(resourceID)
	domain, ok := strings.CutPrefix(resourceID, "domain:")
	if !ok || domain == "" {
		return a
	}
	r, ok := src.(domainOwnerResolver)
	if !ok {
		return a
	}
	owner, found, err := r.DomainAppOwnerOf(ctx, domain)
	// Standalone, parked and static site claims have no app, so those stay admin-only.
	if err != nil || !found || owner == store.StandaloneDomainOwner || owner == store.ParkedDomainOwner {
		return a
	}
	return PushAudience{AppName: owner, AdminOnly: true}
}

// channelProjectGetter is the optional store surface that reads a channel's project.
type channelProjectGetter interface {
	GetNotificationChannel(ctx context.Context, id string) (*NotificationChannel, error)
}

// withChannelProject fills the audience project from the channel, when known.
func withChannelProject(ctx context.Context, src any, channelID string, a PushAudience) PushAudience {
	if channelID == "" || a.AppName == "" {
		return a
	}
	if g, ok := src.(channelProjectGetter); ok {
		if c, err := g.GetNotificationChannel(ctx, channelID); err == nil && c != nil {
			a.ProjectID = c.ProjectID
		}
	}
	return a
}
