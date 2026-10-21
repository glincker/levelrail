package api

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/mailtemplates"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ApprovalChatNotifier is the surface requestDeployApproval needs to
// post an interactive Approve/Deny message. *alerting.DeployDispatcher
// satisfies this structurally, the same "one dispatcher, several narrow
// interfaces" shape NotificationChannelTester already establishes.
type ApprovalChatNotifier interface {
	SendApprovalRequest(ctx context.Context, c alerting.NotificationChannel, info alerting.ApprovalRequestInfo) error
}

// notifyApprovalRequested posts a to every enabled notification channel
// that opted into InteractiveApprovals (Slack/Discord only): best
// effort, logged not returned, the same "a notification failing to send
// must never affect the real operation" reasoning
// alerting.DeployDispatcher.Dispatch's own doc comment gives for a
// deploy outcome. No-op when rt.approvalChatNotifier or
// rt.notificationChannels isn't configured, same "optional signal"
// shape as every other nil-is-valid Router dependency.
func (rt *Router) notifyApprovalRequested(ctx context.Context, a store.DeployApproval) {
	if rt.notificationChannels == nil {
		return
	}
	channels, err := rt.notificationChannels.ListNotificationChannels(ctx)
	if err != nil {
		rt.logger.Error("api: list notification channels for deploy approval failed", slog.String("error", err.Error()), slog.String("approval_id", a.ID))
		return
	}
	info := alerting.ApprovalRequestInfo{
		ApprovalID: a.ID, ServiceName: a.ServiceName, Action: a.Action,
		Image: a.Image, RequestedBy: a.RequestedByName, EnvironmentID: a.EnvironmentID,
	}
	for _, c := range channels {
		if !c.Enabled || !c.InteractiveApprovals {
			continue
		}
		if c.Kind == alerting.NotifyEmail {
			rt.emailApprovalRequest(ctx, c, a)
			continue
		}
		if rt.approvalChatNotifier == nil {
			continue
		}
		if err := rt.approvalChatNotifier.SendApprovalRequest(ctx, c, info); err != nil {
			rt.logger.Error("api: send deploy approval chat request failed",
				slog.String("channel_id", c.ID), slog.String("approval_id", a.ID), slog.String("error", err.Error()))
		}
	}
}

// emailApprovalRequest sends the deploy_approval template to an email channel, best effort.
func (rt *Router) emailApprovalRequest(ctx context.Context, c alerting.NotificationChannel, a store.DeployApproval) {
	if alerting.IsMaskedNotifyTarget(c.NotifyURL) || c.NotifyURL == "" {
		return
	}
	vals := map[string]string{"app_name": a.ServiceName, "requester": a.RequestedByName, "approve_link": rt.emailLinkBase(ctx)}
	if err := rt.sendMailEvent(ctx, mailtemplates.EventDeployApproval, c.NotifyURL, vals); err != nil {
		rt.logger.Error("api: email deploy approval request failed",
			slog.String("channel_id", c.ID), slog.String("approval_id", a.ID), slog.String("error", err.Error()))
	}
}
