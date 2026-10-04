package alerting

import (
	"context"
	"fmt"
	"log/slog"
)

// This file sends the one chat-interactive message this package knows
// about: a pending deploy approval, posted to every enabled channel
// that opted into InteractiveApprovals (notification_channel.go), with
// real Approve/Deny buttons so an approver can act from their phone.
// internal/api/chat_interactions.go is the other half: it receives and
// verifies the button click this message produces.

// ApprovalRequestInfo is what SendApprovalRequest needs to render an
// interactive message: just enough to identify the gated change and
// let an approver decide without opening the dashboard. ApprovalID is
// the value every button carries back on click (deploy_approval.go's
// own opaque ID), never anything an approver would need to trust blind.
type ApprovalRequestInfo struct {
	ApprovalID  string
	ServiceName string
	// Action is one of store.DeployApprovalAction* ("deploy" or
	// "promote"), duplicated here as a plain string so this package
	// doesn't need to import internal/store just for two constants.
	Action        string
	Image         string
	RequestedBy   string
	EnvironmentID string
}

// approvalIntroText is the one line both Slack and Discord payloads
// lead with.
func approvalIntroText(info ApprovalRequestInfo) string {
	verb := "Deploy"
	if info.Action == "promote" {
		verb = "Promote"
	}
	return fmt.Sprintf("*%s approval requested*\n%s -> `%s`\nRequested by %s", verb, info.ServiceName, info.Image, info.RequestedBy)
}

// slackApprovalPayload is Slack's incoming-webhook shape with Block
// Kit's "actions" block added: Slack renders a real section plus two
// buttons from this, and (because the webhook belongs to an app with
// Interactivity enabled, not a bare "custom integration" webhook) a
// click posts to that app's own configured Request URL, verified by
// chat_interactions.go's handleSlackInteraction.
type slackApprovalPayload struct {
	Text   string             `json:"text"`
	Blocks []slackApprovalBlk `json:"blocks"`
}

type slackApprovalBlk struct {
	Type     string                `json:"type"`
	Text     *slackApprovalText    `json:"text,omitempty"`
	Elements []slackApprovalButton `json:"elements,omitempty"`
}

type slackApprovalText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type slackApprovalButton struct {
	Type     string             `json:"type"`
	Text     slackApprovalText  `json:"text"`
	Style    string             `json:"style,omitempty"`
	ActionID string             `json:"action_id"`
	Value    string             `json:"value"`
	Confirm  *slackApprovalConf `json:"confirm,omitempty"`
}

type slackApprovalConf struct {
	Title   slackApprovalText `json:"title"`
	Text    slackApprovalText `json:"text"`
	Confirm slackApprovalText `json:"confirm"`
	Deny    slackApprovalText `json:"deny"`
}

// SlackApprovalActionApprove and SlackApprovalActionReject are the
// action_id every Approve/Deny button carries, and the one shared
// constant internal/api/chat_interactions.go dispatches on: a repeated
// string literal for the same action_id in two packages would risk the
// button and the handler silently drifting apart.
const (
	SlackApprovalActionApprove = "deploy_approval_approve"
	SlackApprovalActionReject  = "deploy_approval_reject"
)

func buildSlackApprovalPayload(info ApprovalRequestInfo) slackApprovalPayload {
	text := approvalIntroText(info)
	return slackApprovalPayload{
		Text: text,
		Blocks: []slackApprovalBlk{
			{Type: "section", Text: &slackApprovalText{Type: "mrkdwn", Text: text}},
			{Type: "actions", Elements: []slackApprovalButton{
				{
					Type: "button", Text: slackApprovalText{Type: "plain_text", Text: "Approve"},
					Style: "primary", ActionID: SlackApprovalActionApprove, Value: info.ApprovalID,
				},
				{
					Type: "button", Text: slackApprovalText{Type: "plain_text", Text: "Deny"},
					Style: "danger", ActionID: SlackApprovalActionReject, Value: info.ApprovalID,
					Confirm: &slackApprovalConf{
						Title:   slackApprovalText{Type: "plain_text", Text: "Deny this deploy?"},
						Text:    slackApprovalText{Type: "plain_text", Text: "This cannot be undone from here."},
						Confirm: slackApprovalText{Type: "plain_text", Text: "Deny"},
						Deny:    slackApprovalText{Type: "plain_text", Text: "Cancel"},
					},
				},
			}},
		},
	}
}

// discordApprovalPayload is Discord's webhook-execute shape with a
// message-components action row added (components only render when the
// webhook's own application has an Interactions Endpoint URL
// configured; see docs/chat-deploy-approvals.md).
type discordApprovalPayload struct {
	Content    string               `json:"content"`
	Components []discordApprovalRow `json:"components"`
}

type discordApprovalRow struct {
	Type       int                  `json:"type"`
	Components []discordApprovalBtn `json:"components"`
}

type discordApprovalBtn struct {
	Type     int    `json:"type"`
	Style    int    `json:"style"`
	Label    string `json:"label"`
	CustomID string `json:"custom_id"`
}

// Discord component/button type and style constants
// (discord.com/developers/docs/interactions/message-components).
const (
	discordComponentTypeActionRow = 1
	discordComponentTypeButton    = 2
	discordButtonStyleSuccess     = 3
	discordButtonStyleDanger      = 4
)

// DiscordApprovalCustomIDApprove/Reject prefix a button's custom_id;
// chat_interactions.go splits on ":" to recover the approval ID.
const (
	DiscordApprovalCustomIDApprove = "deploy_approval_approve"
	DiscordApprovalCustomIDReject  = "deploy_approval_reject"
)

func buildDiscordApprovalPayload(info ApprovalRequestInfo) discordApprovalPayload {
	return discordApprovalPayload{
		Content: approvalIntroText(info),
		Components: []discordApprovalRow{{
			Type: discordComponentTypeActionRow,
			Components: []discordApprovalBtn{
				{Type: discordComponentTypeButton, Style: discordButtonStyleSuccess, Label: "Approve", CustomID: DiscordApprovalCustomIDApprove + ":" + info.ApprovalID},
				{Type: discordComponentTypeButton, Style: discordButtonStyleDanger, Label: "Deny", CustomID: DiscordApprovalCustomIDReject + ":" + info.ApprovalID},
			},
		}},
	}
}

// ErrInteractiveApprovalUnsupported is returned by SendApprovalRequest
// for any channel kind other than Slack or Discord: InteractiveApprovals
// never applies to one (see notificationChannelsSupportingInteractiveApprovals
// in internal/api/notification_channels.go, the matching validation at
// save time), so reaching this in practice means a row was written
// before that validation existed.
var ErrInteractiveApprovalUnsupported = fmt.Errorf("alerting: send approval request: channel kind does not support interactive approval")

// SendApprovalRequest posts one deploy-approval-requested message to c,
// with real Approve/Deny buttons for NotifySlack and NotifyDiscord.
// Records delivery history the same way Dispatch does for a deploy
// outcome, under trigger "deploy-approval-requested".
func (d *DeployDispatcher) SendApprovalRequest(ctx context.Context, c NotificationChannel, info ApprovalRequestInfo) error {
	var sendErr error
	switch c.Kind {
	case NotifySlack:
		sendErr = postJSON(ctx, d.client, c.NotifyURL, buildSlackApprovalPayload(info))
	case NotifyDiscord:
		sendErr = postJSON(ctx, d.client, c.NotifyURL, buildDiscordApprovalPayload(info))
	default:
		sendErr = ErrInteractiveApprovalUnsupported
	}
	recordDelivery(ctx, d.db, d.logger, c.ID, "deploy-approval-requested", sendErr)
	if sendErr != nil {
		d.logger.Error("alerting: send deploy approval request failed",
			slog.String("channel_id", c.ID), slog.String("approval_id", info.ApprovalID), slog.String("error", sendErr.Error()))
	}
	return sendErr
}
