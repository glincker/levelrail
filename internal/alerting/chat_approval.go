package alerting

import (
	"context"
	"fmt"
	"log/slog"
)

// Sends the interactive deploy-approval message (Approve/Deny buttons) to channels that opted in.

// ApprovalRequestInfo identifies the gated change and the approval ID each button carries back on click.
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

// Slack webhook shape plus a Block Kit actions block. Clicks post to the app's Request URL, verified in chat_interactions.go.
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

// Shared action_id constants, so the Slack buttons and their handler in internal/api cannot drift apart.
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

// Discord webhook shape plus an action row, which only renders when the webhook app has an Interactions Endpoint URL.
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

// ErrInteractiveApprovalUnsupported is returned for channel kinds other than Slack or Discord.
// Reaching it means the row predates save-time validation.
var ErrInteractiveApprovalUnsupported = fmt.Errorf("alerting: send approval request: channel kind does not support interactive approval")

// SendApprovalRequest posts a deploy-approval request with Approve/Deny buttons to c.
// Delivery history is recorded under trigger deploy-approval-requested.
func (d *DeployDispatcher) SendApprovalRequest(ctx context.Context, c NotificationChannel, info ApprovalRequestInfo) error {
	revealed, sendErr := d.db.RevealNotificationChannel(ctx, c)
	switch {
	case sendErr != nil:
	case c.Kind == NotifySlack:
		sendErr = postJSON(ctx, d.client, revealed.NotifyURL, buildSlackApprovalPayload(info))
	case c.Kind == NotifyDiscord:
		sendErr = postJSON(ctx, d.client, revealed.NotifyURL, buildDiscordApprovalPayload(info))
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
