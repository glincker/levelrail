package api

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// This file receives the Slack/Discord button clicks
// alerting/chat_approval.go sends. Unauthenticated by session or token
// like handleGitPushWebhook (router.go); a verified click's own
// signature check stands in for auth, see docs/chat-deploy-approvals.md.
// A verified click still only ever decides through
// loadDecidableApprovalAs/applyAndDecideApproval/rejectApproval
// (deploy_approvals.go), the exact functions the dashboard's own
// session/token-gated handlers call.

// chatApprovalActorType marks a decision as made via a verified chat
// button, not a Levelrail user or token: Levelrail has no mechanism
// linking a Slack/Discord user to an account, so a verified click is
// authorized at the channel level, not the individual level (see
// docs/chat-deploy-approvals.md for the full scope statement). The
// same-actor check still applies, compared against this synthetic
// (type, channel ID) pair.
const chatApprovalActorType = "channel"

// chatInteractionMaxBodyBytes bounds how much of a Slack/Discord
// interaction request this handler reads, the same DoS-shaped guard
// webhook.MaxPayloadBytes already gives the git push webhook: an
// interaction payload is small (a button click, not a diff), so this
// just reuses that existing limit rather than defining a second one.
const chatInteractionMaxBodyBytes = webhook.MaxPayloadBytes

// slackSignatureTolerance rejects a Slack interaction signed more than
// this long ago (or, implausibly, in the future): Slack's own
// documented replay-protection window
// (api.slack.com/authentication/verifying-requests-from-slack).
const slackSignatureTolerance = 5 * time.Minute

// readChatInteractionBody reads and size-limits r.Body the same way
// handleGitPushWebhook does, shared by both platform handlers below.
func readChatInteractionBody(r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, chatInteractionMaxBodyBytes+1))
	if err != nil {
		return nil, false
	}
	return body, len(body) <= chatInteractionMaxBodyBytes
}

// interactiveChannelsOf returns every enabled channel of kind with a
// non-empty InteractiveSecret: the candidate pool both
// verifySlackChannelSignature and verifyDiscordChannelSignature try
// against, since neither platform's request identifies which Levelrail
// channel (and therefore which secret) sent the original message before
// the signature is itself verified.
func interactiveChannelsOf(channels []alerting.NotificationChannel, kind alerting.NotifyKind) []alerting.NotificationChannel {
	out := make([]alerting.NotificationChannel, 0, len(channels))
	for _, c := range channels {
		if c.Enabled && c.InteractiveApprovals && c.Kind == kind && c.InteractiveSecret != "" {
			out = append(out, c)
		}
	}
	return out
}

// verifySlackSignature checks sigHeader (X-Slack-Signature) against an
// HMAC-SHA256 of "v0:<timestamp>:<body>" keyed by secret
// (api.slack.com/authentication/verifying-requests-from-slack), the
// identical hmac.Equal constant-time comparison
// internal/webhook.VerifySignature already uses for GitHub's analogous
// scheme, and rejects a timestamp outside slackSignatureTolerance.
func verifySlackSignature(secret, timestamp, body, sigHeader string) bool {
	if secret == "" || timestamp == "" || sigHeader == "" {
		return false
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if age := time.Since(time.Unix(ts, 0)); age > slackSignatureTolerance || age < -slackSignatureTolerance {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + timestamp + ":" + body))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sigHeader))
}

// verifySlackChannelSignature tries sigHeader against every Slack
// candidate channel's own secret, returning the first match. Fails
// closed (ok=false) when none verifies, including when there are no
// candidate channels at all.
func verifySlackChannelSignature(channels []alerting.NotificationChannel, timestamp, body, sigHeader string) (alerting.NotificationChannel, bool) {
	for _, c := range channels {
		if verifySlackSignature(c.InteractiveSecret, timestamp, body, sigHeader) {
			return c, true
		}
	}
	return alerting.NotificationChannel{}, false
}

// verifyDiscordSignature checks sigHeader (X-Signature-Ed25519) as an
// Ed25519 signature over timestamp+body, keyed by publicKeyHex
// (discord.com/developers/docs/interactions/receiving-and-responding):
// ed25519.Verify is constant-time by construction, the same property
// hmac.Equal gives verifySlackSignature above.
func verifyDiscordSignature(publicKeyHex, timestamp string, body []byte, sigHeader string) bool {
	if publicKeyHex == "" || timestamp == "" || sigHeader == "" {
		return false
	}
	pub, err := hex.DecodeString(publicKeyHex)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(sigHeader)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	msg := append([]byte(timestamp), body...)
	return ed25519.Verify(ed25519.PublicKey(pub), msg, sig)
}

// verifyDiscordChannelSignature is verifySlackChannelSignature's
// Discord counterpart.
func verifyDiscordChannelSignature(channels []alerting.NotificationChannel, timestamp string, body []byte, sigHeader string) (alerting.NotificationChannel, bool) {
	for _, c := range channels {
		if verifyDiscordSignature(c.InteractiveSecret, timestamp, body, sigHeader) {
			return c, true
		}
	}
	return alerting.NotificationChannel{}, false
}

// slackInteractionPayload is the subset of Slack's block_actions
// interaction payload this handler needs
// (api.slack.com/reference/interaction-payloads/block-actions).
type slackInteractionPayload struct {
	Type    string `json:"type"`
	Actions []struct {
		ActionID string `json:"action_id"`
		Value    string `json:"value"`
	} `json:"actions"`
	User struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
}

// handleSlackInteraction handles
// POST /api/v1/webhooks/slack/interactions: Slack's own Interactivity
// Request URL for every Block Kit button click across every Slack app
// connected to this control plane, not just deploy approvals, though
// today this is the only kind of interactive message this codebase
// ever sends. Deliberately unauthenticated by session or token (see
// this file's own doc comment); X-Slack-Signature verification against
// a channel's own InteractiveSecret is what stands in for auth.
func (rt *Router) handleSlackInteraction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if rt.webhookRateLimit != nil {
		if allowed, retryAfter := rt.webhookRateLimit.allow(clientIP(r) + "|slack-interactions"); !allowed {
			writeRateLimited(w, retryAfter)
			return
		}
	}
	if rt.notificationChannels == nil {
		writeError(w, http.StatusNotImplemented, "notification channels are not configured on this control plane")
		return
	}

	body, ok := readChatInteractionBody(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "request body too large")
		return
	}

	channels, err := rt.notificationChannels.ListNotificationChannels(r.Context())
	if err != nil {
		rt.internalError(w, "api: slack interaction: list notification channels failed", err)
		return
	}
	candidates := interactiveChannelsOf(channels, alerting.NotifySlack)
	channel, ok := verifySlackChannelSignature(candidates, r.Header.Get("X-Slack-Request-Timestamp"), string(body), r.Header.Get("X-Slack-Signature"))
	if !ok {
		rt.logger.Warn("api: slack interaction: signature verification failed", slog.String("remote_addr", r.RemoteAddr))
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	form, err := url.ParseQuery(string(body))
	if err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}
	var payload slackInteractionPayload
	if err := json.Unmarshal([]byte(form.Get("payload")), &payload); err != nil {
		writeError(w, http.StatusBadRequest, "malformed interaction payload")
		return
	}
	if payload.Type != "block_actions" || len(payload.Actions) == 0 {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "Nothing to do."})
		return
	}

	action := payload.Actions[0]
	deciderName := fmt.Sprintf("Slack:%s (clicked by %s)", channel.Name, payload.User.Username)
	text := rt.decideChatApproval(r.Context(), action.ActionID == alerting.SlackApprovalActionApprove, action.Value, channel.ID, deciderName)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"text":             text,
		"replace_original": true,
		"response_type":    "in_channel",
	})
}

// discordInteractionPayload is the subset of Discord's interaction
// payload this handler needs
// (discord.com/developers/docs/interactions/receiving-and-responding),
// covering both PING (type 1, endpoint verification) and
// MESSAGE_COMPONENT (type 3, a button click). Member.User is set for a
// guild interaction, User for a DM one; Discord sends exactly one of
// the two.
type discordInteractionPayload struct {
	Type int `json:"type"`
	Data struct {
		CustomID string `json:"custom_id"`
	} `json:"data"`
	Member struct {
		User discordInteractionUser `json:"user"`
	} `json:"member"`
	User discordInteractionUser `json:"user"`
}

type discordInteractionUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// Discord interaction/response type constants this handler needs
// (discord.com/developers/docs/interactions/receiving-and-responding).
const (
	discordInteractionTypePing             = 1
	discordInteractionTypeMessageComponent = 3
	discordResponseTypePong                = 1
	discordResponseTypeUpdateMessage       = 7
)

// handleDiscordInteraction handles
// POST /api/v1/webhooks/discord/interactions: Discord's own
// Interactions Endpoint URL, for both the PING Discord sends once when
// the URL is first saved (must be answered correctly or Discord refuses
// to save it) and every subsequent button click. Unauthenticated like
// this file's own doc comment describes; Ed25519-verified against a
// channel's own InteractiveSecret (the Discord application's public
// key) instead.
func (rt *Router) handleDiscordInteraction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if rt.webhookRateLimit != nil {
		if allowed, retryAfter := rt.webhookRateLimit.allow(clientIP(r) + "|discord-interactions"); !allowed {
			writeRateLimited(w, retryAfter)
			return
		}
	}
	if rt.notificationChannels == nil {
		writeError(w, http.StatusNotImplemented, "notification channels are not configured on this control plane")
		return
	}

	body, ok := readChatInteractionBody(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "request body too large")
		return
	}

	channels, err := rt.notificationChannels.ListNotificationChannels(r.Context())
	if err != nil {
		rt.internalError(w, "api: discord interaction: list notification channels failed", err)
		return
	}
	candidates := interactiveChannelsOf(channels, alerting.NotifyDiscord)
	channel, ok := verifyDiscordChannelSignature(candidates, r.Header.Get("X-Signature-Timestamp"), body, r.Header.Get("X-Signature-Ed25519"))
	if !ok {
		rt.logger.Warn("api: discord interaction: signature verification failed", slog.String("remote_addr", r.RemoteAddr))
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	var payload discordInteractionPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "malformed interaction payload")
		return
	}

	if payload.Type == discordInteractionTypePing {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]int{"type": discordResponseTypePong})
		return
	}
	if payload.Type != discordInteractionTypeMessageComponent {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]int{"type": discordResponseTypePong})
		return
	}

	user := payload.Member.User
	if user.ID == "" {
		user = payload.User
	}
	actionPrefix, approvalID, ok := strings.Cut(payload.Data.CustomID, ":")
	if !ok {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(discordUpdateMessage("This button is no longer valid."))
		return
	}

	deciderName := fmt.Sprintf("Discord:%s (clicked by %s)", channel.Name, user.Username)
	text := rt.decideChatApproval(r.Context(), actionPrefix == alerting.DiscordApprovalCustomIDApprove, approvalID, channel.ID, deciderName)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(discordUpdateMessage(text))
}

// discordUpdateMessage builds an UPDATE_MESSAGE response that replaces
// the original message's content and removes its buttons: once an
// approval is decided, re-clicking (or a double-click race, already
// made safe by DecideDeployApproval's own compare-and-swap) has nothing
// left to act on.
func discordUpdateMessage(content string) map[string]any {
	return map[string]any{
		"type": discordResponseTypeUpdateMessage,
		"data": map[string]any{"content": content, "components": []any{}},
	}
}

// decideChatApproval is the one function both handleSlackInteraction
// and handleDiscordInteraction call to decide approvalID, through the
// same loadDecidableApprovalAs/applyAndDecideApproval/rejectApproval
// functions the dashboard's own handlers call. Returns the outcome text
// the caller posts back, never an HTTP status: neither platform's
// response reaches the original requester.
func (rt *Router) decideChatApproval(ctx context.Context, approve bool, approvalID, channelID, deciderName string) string {
	a, err := rt.loadDecidableApprovalAs(ctx, approvalID, chatApprovalActorType, channelID)
	if err != nil {
		return err.Error()
	}

	if approve {
		resp, err := rt.applyAndDecideApproval(ctx, a, chatApprovalActorType, channelID, deciderName)
		if err != nil {
			return "Could not approve: " + err.Error()
		}
		return fmt.Sprintf("Approved by %s. %s now targets %s.", deciderName, resp.Approval.ServiceName, resp.Approval.Image)
	}

	resource, err := rt.rejectApproval(ctx, approvalID, chatApprovalActorType, channelID, deciderName, "denied via chat")
	if err != nil {
		return "Could not deny: " + err.Error()
	}
	return fmt.Sprintf("Denied by %s. %s was not deployed.", deciderName, resource.ServiceName)
}
