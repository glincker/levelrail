package api

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// --- pure signature-verification unit tests ---

func TestVerifySlackSignature(t *testing.T) {
	secret := "shh-its-a-secret"
	body := `payload=%7B%22type%22%3A%22block_actions%22%7D`
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := slackTestSignature(secret, ts, body)

	if !verifySlackSignature(secret, ts, body, sig) {
		t.Fatal("a correctly computed signature must verify")
	}
	if verifySlackSignature("wrong-secret", ts, body, sig) {
		t.Fatal("a signature computed with a different secret must not verify")
	}
	if verifySlackSignature(secret, ts, body+"tampered", sig) {
		t.Fatal("a signature over a different body must not verify")
	}
	if verifySlackSignature(secret, "", body, sig) {
		t.Fatal("an empty timestamp must not verify")
	}
	staleTS := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	staleSig := slackTestSignature(secret, staleTS, body)
	if verifySlackSignature(secret, staleTS, body, staleSig) {
		t.Fatal("a signature older than slackSignatureTolerance must not verify")
	}
}

func TestVerifyDiscordSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pubHex := hex.EncodeToString(pub)
	body := []byte(`{"type":3,"data":{"custom_id":"deploy_approval_approve:apr_x"}}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := hex.EncodeToString(ed25519.Sign(priv, append([]byte(ts), body...)))

	if !verifyDiscordSignature(pubHex, ts, body, sig) {
		t.Fatal("a correctly computed Ed25519 signature must verify")
	}
	otherPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate second key: %v", err)
	}
	if verifyDiscordSignature(hex.EncodeToString(otherPub), ts, body, sig) {
		t.Fatal("a signature checked against a different public key must not verify")
	}
	if verifyDiscordSignature(pubHex, ts, append(body, 'x'), sig) {
		t.Fatal("a signature over a different body must not verify")
	}
	if verifyDiscordSignature(pubHex, ts, body, hex.EncodeToString([]byte("not-a-real-signature-of-the-right-length-000000"))) {
		t.Fatal("a malformed signature must not verify")
	}
}

// slackTestSignature mirrors verifySlackSignature's own HMAC
// construction, standing in for a real Slack request in these tests.
func slackTestSignature(secret, timestamp, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + timestamp + ":" + body))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

// --- end-to-end handler tests ---

// seedInteractiveSlackChannel saves and returns a Slack notification
// channel with interactive approvals enabled, secret, for the handler
// tests below to sign requests against.
func seedInteractiveSlackChannel(t *testing.T, adb *alerting.DB, secret string) {
	t.Helper()
	c := alerting.NotificationChannel{
		ID: "chn_slack_test", Name: "team-slack", Kind: alerting.NotifySlack,
		NotifyURL: "https://hooks.slack.com/services/x", Enabled: true,
		InteractiveApprovals: true, InteractiveSecret: secret,
	}
	if err := adb.SaveNotificationChannel(context.Background(), c); err != nil {
		t.Fatalf("seed slack channel: %v", err)
	}
}

func seedInteractiveDiscordChannel(t *testing.T, adb *alerting.DB, publicKeyHex string) {
	t.Helper()
	c := alerting.NotificationChannel{
		ID: "chn_discord_test", Name: "ops-discord", Kind: alerting.NotifyDiscord,
		NotifyURL: "https://discord.com/api/webhooks/x/y", Enabled: true,
		InteractiveApprovals: true, InteractiveSecret: publicKeyHex,
	}
	if err := adb.SaveNotificationChannel(context.Background(), c); err != nil {
		t.Fatalf("seed discord channel: %v", err)
	}
}

func slackInteractionRequest(t *testing.T, secret, actionID, approvalID string) *http.Request {
	t.Helper()
	payload := fmt.Sprintf(`{"type":"block_actions","actions":[{"action_id":%q,"value":%q}],"user":{"id":"U123","username":"alice"}}`, actionID, approvalID)
	body := "payload=" + url.QueryEscape(payload)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := slackTestSignature(secret, ts, body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/slack/interactions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", sig)
	return req
}

func discordInteractionRequest(t *testing.T, priv ed25519.PrivateKey, customID string) *http.Request {
	t.Helper()
	body := []byte(fmt.Sprintf(`{"type":3,"data":{"custom_id":%q},"member":{"user":{"id":"D123","username":"bob"}}}`, customID))
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := hex.EncodeToString(ed25519.Sign(priv, append([]byte(ts), body...)))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/discord/interactions", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature-Timestamp", ts)
	req.Header.Set("X-Signature-Ed25519", sig)
	return req
}

// newTestRouterForChatApprovals wires notification channels (needed by
// both the chat-interaction handlers and requestDeployApproval's own
// best-effort chat notify) the same way newTestRouterWithNotificationChannels
// does, reused here under this file's own name for clarity at call sites.
func newTestRouterForChatApprovals(t *testing.T) (*Router, *store.DB, *alerting.DB) {
	return newTestRouterWithNotificationChannels(t)
}

func TestHandleSlackInteraction_ApprovesDeploy(t *testing.T) {
	rt, db, adb := newTestRouterForChatApprovals(t)
	requester := loginTestSession(t, rt, db)
	seedProtectedDeployFixture(t, db)
	const secret = "slack-signing-secret"
	seedInteractiveSlackChannel(t, adb, secret)

	id := requestPendingApproval(t, rt, requester)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, slackInteractionRequest(t, secret, alerting.SlackApprovalActionApprove, id))
	if rec.Code != http.StatusOK {
		t.Fatalf("slack approve: status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	a, err := db.GetDeployApproval(context.Background(), id)
	if err != nil {
		t.Fatalf("GetDeployApproval: %v", err)
	}
	if a.Status != store.DeployApprovalStatusApproved {
		t.Errorf("Status = %q, want %q", a.Status, store.DeployApprovalStatusApproved)
	}
	if a.ApprovedByType != chatApprovalActorType {
		t.Errorf("ApprovedByType = %q, want %q", a.ApprovedByType, chatApprovalActorType)
	}

	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	if svc.Image != requestPendingApprovalImage {
		t.Errorf("a chat-approved deploy must actually update desired state, Image = %q", svc.Image)
	}
}

func TestHandleSlackInteraction_DeniesDeploy(t *testing.T) {
	rt, db, adb := newTestRouterForChatApprovals(t)
	requester := loginTestSession(t, rt, db)
	seedProtectedDeployFixture(t, db)
	const secret = "slack-signing-secret"
	seedInteractiveSlackChannel(t, adb, secret)

	id := requestPendingApproval(t, rt, requester)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, slackInteractionRequest(t, secret, alerting.SlackApprovalActionReject, id))
	if rec.Code != http.StatusOK {
		t.Fatalf("slack deny: status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	a, err := db.GetDeployApproval(context.Background(), id)
	if err != nil {
		t.Fatalf("GetDeployApproval: %v", err)
	}
	if a.Status != store.DeployApprovalStatusRejected {
		t.Errorf("Status = %q, want %q", a.Status, store.DeployApprovalStatusRejected)
	}

	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	if svc.Image != "levelrail/web:1" {
		t.Errorf("a denied deploy must never proceed, Image = %q", svc.Image)
	}
}

// TestHandleSlackInteraction_InvalidSignatureRejected proves an
// unverified request is rejected outright, with no state change: the
// central security guarantee this whole feature rests on.
func TestHandleSlackInteraction_InvalidSignatureRejected(t *testing.T) {
	rt, db, adb := newTestRouterForChatApprovals(t)
	requester := loginTestSession(t, rt, db)
	seedProtectedDeployFixture(t, db)
	seedInteractiveSlackChannel(t, adb, "the-real-secret")

	id := requestPendingApproval(t, rt, requester)

	// Signed with the wrong secret: verifySlackChannelSignature must
	// fail to match any configured channel.
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, slackInteractionRequest(t, "an-attackers-guess", alerting.SlackApprovalActionApprove, id))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("forged signature: status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	a, err := db.GetDeployApproval(context.Background(), id)
	if err != nil {
		t.Fatalf("GetDeployApproval: %v", err)
	}
	if a.Status != store.DeployApprovalStatusPending {
		t.Errorf("an unverified request must never decide the approval, Status = %q", a.Status)
	}
	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	if svc.Image != "levelrail/web:1" {
		t.Errorf("an unverified request must never change desired state, Image = %q", svc.Image)
	}
}

// TestHandleSlackInteraction_ReplayCannotDoubleApprove proves a second,
// identically-valid interaction for an already-decided approval cannot
// apply again: DecideDeployApproval's own compare-and-swap
// (WHERE status = 'pending') is what actually enforces this, exercised
// here through the real HTTP path a replayed or retried Slack delivery
// would take.
func TestHandleSlackInteraction_ReplayCannotDoubleApprove(t *testing.T) {
	rt, db, adb := newTestRouterForChatApprovals(t)
	requester := loginTestSession(t, rt, db)
	seedProtectedDeployFixture(t, db)
	const secret = "slack-signing-secret"
	seedInteractiveSlackChannel(t, adb, secret)

	id := requestPendingApproval(t, rt, requester)

	first := httptest.NewRecorder()
	rt.Handler().ServeHTTP(first, slackInteractionRequest(t, secret, alerting.SlackApprovalActionApprove, id))
	if first.Code != http.StatusOK {
		t.Fatalf("first approve: status = %d, want %d; body = %s", first.Code, http.StatusOK, first.Body.String())
	}

	attemptsAfterFirst, err := db.ListDeployAttempts(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListDeployAttempts: %v", err)
	}

	second := httptest.NewRecorder()
	rt.Handler().ServeHTTP(second, slackInteractionRequest(t, secret, alerting.SlackApprovalActionApprove, id))
	// The replay is still a verified, well-formed interaction, so the
	// handler responds 200 (there is no HTTP-status channel back to the
	// original approver to retry against); what must not happen is a
	// second apply.
	if second.Code != http.StatusOK {
		t.Fatalf("replayed approve: status = %d, want %d; body = %s", second.Code, http.StatusOK, second.Body.String())
	}
	var replayResp map[string]any
	if err := json.Unmarshal(second.Body.Bytes(), &replayResp); err != nil {
		t.Fatalf("decode replay response: %v", err)
	}
	if !strings.Contains(fmt.Sprint(replayResp["text"]), "no longer pending") {
		t.Errorf("replay response text = %v, want it to say the approval is no longer pending", replayResp["text"])
	}

	attemptsAfterSecond, err := db.ListDeployAttempts(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListDeployAttempts: %v", err)
	}
	if len(attemptsAfterSecond) != len(attemptsAfterFirst) {
		t.Errorf("a replayed approve must not re-apply: deploy attempts went from %d to %d", len(attemptsAfterFirst), len(attemptsAfterSecond))
	}
}

func TestHandleDiscordInteraction_Ping(t *testing.T) {
	rt, _, adb := newTestRouterForChatApprovals(t)
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	seedInteractiveDiscordChannel(t, adb, hex.EncodeToString(pub))

	body := []byte(`{"type":1}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := hex.EncodeToString(ed25519.Sign(priv, append([]byte(ts), body...)))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/discord/interactions", strings.NewReader(string(body)))
	req.Header.Set("X-Signature-Timestamp", ts)
	req.Header.Set("X-Signature-Ed25519", sig)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ping: status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode ping response: %v", err)
	}
	if resp["type"] != discordResponseTypePong {
		t.Errorf("response type = %d, want %d (PONG)", resp["type"], discordResponseTypePong)
	}
}

func TestHandleDiscordInteraction_ApprovesDeploy(t *testing.T) {
	rt, db, adb := newTestRouterForChatApprovals(t)
	requester := loginTestSession(t, rt, db)
	seedProtectedDeployFixture(t, db)
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	seedInteractiveDiscordChannel(t, adb, hex.EncodeToString(pub))

	id := requestPendingApproval(t, rt, requester)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, discordInteractionRequest(t, priv, alerting.DiscordApprovalCustomIDApprove+":"+id))
	if rec.Code != http.StatusOK {
		t.Fatalf("discord approve: status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	a, err := db.GetDeployApproval(context.Background(), id)
	if err != nil {
		t.Fatalf("GetDeployApproval: %v", err)
	}
	if a.Status != store.DeployApprovalStatusApproved {
		t.Errorf("Status = %q, want %q", a.Status, store.DeployApprovalStatusApproved)
	}
	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	if svc.Image != requestPendingApprovalImage {
		t.Errorf("a chat-approved deploy must actually update desired state, Image = %q", svc.Image)
	}
}

func TestHandleDiscordInteraction_InvalidSignatureRejected(t *testing.T) {
	rt, db, adb := newTestRouterForChatApprovals(t)
	requester := loginTestSession(t, rt, db)
	seedProtectedDeployFixture(t, db)
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	seedInteractiveDiscordChannel(t, adb, hex.EncodeToString(pub))
	_, attackerPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate attacker key: %v", err)
	}

	id := requestPendingApproval(t, rt, requester)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, discordInteractionRequest(t, attackerPriv, alerting.DiscordApprovalCustomIDApprove+":"+id))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("forged signature: status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	a, err := db.GetDeployApproval(context.Background(), id)
	if err != nil {
		t.Fatalf("GetDeployApproval: %v", err)
	}
	if a.Status != store.DeployApprovalStatusPending {
		t.Errorf("an unverified request must never decide the approval, Status = %q", a.Status)
	}
}
