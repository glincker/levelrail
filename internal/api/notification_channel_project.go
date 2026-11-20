package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// validateChannelProject checks a channel's project through the same team helper every project filing uses.
func (rt *Router) validateChannelProject(ctx context.Context, projectID string) error {
	if projectID == "" {
		return nil
	}
	if db := rt.teamDB(); db != nil {
		if _, err := db.TeamOfProject(ctx, projectID); errors.Is(err, store.ErrProjectNotFound) {
			return errors.New("unknown project_id")
		} else if err != nil {
			return err
		}
	}
	return rt.authorizeProjectAssignment(ctx, projectID)
}

// channelServesApp reports whether a channel may carry alerts of the app. A project channel serves only apps filed under its project.
func (rt *Router) channelServesApp(ctx context.Context, c *alerting.NotificationChannel, appName string) (bool, error) {
	if c.ProjectID == "" {
		return true, nil
	}
	owner, err := rt.ownerForResource(ctx, resourcePrefixApp, appName)
	if err != nil {
		return false, err
	}
	return owner != nil && owner.ProjectID == c.ProjectID, nil
}

// requireChannelForApp loads a channel for a rule or deploy target. Another project's channel answers like a missing one.
func (rt *Router) requireChannelForApp(w http.ResponseWriter, r *http.Request, channelID, appName, op string) bool {
	c, err := rt.notificationChannels.GetNotificationChannel(r.Context(), channelID)
	if errors.Is(err, alerting.ErrNotificationChannelNotFound) {
		writeError(w, http.StatusBadRequest, "unknown channel_id")
		return false
	}
	if err != nil {
		rt.internalError(w, "api: "+op+": look up channel failed", err)
		return false
	}
	ok, err := rt.channelServesApp(r.Context(), c, appName)
	if err != nil {
		rt.internalError(w, "api: "+op+": resolve app project failed", err)
		return false
	}
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown channel_id")
		return false
	}
	return true
}
