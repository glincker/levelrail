package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/eventhook"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

const (
	auditStackSyncWebhookRotate = "stack.sync_webhook_secret_rotated"
	stackSyncWebhookSecretKey   = "webhook_secret"
	eventStackSyncTriggered     = "stacks.sync_triggered"
	// decoy keeps the refused paths doing the same HMAC work as a real check.
	stackSyncWebhookDecoy = "stack-sync-webhook-decoy-never-stored"
)

type stackSyncWebhookSecretOutput struct {
	Secret     string `json:"secret"`
	SyncURL    string `json:"sync_url"`
	RefreshURL string `json:"refresh_url"`
}

// handleRotateStackSyncWebhookSecret handles POST /api/v1/stacks/{name}/sync/webhook-secret/rotate.
func (rt *Router) handleRotateStackSyncWebhookSecret(w http.ResponseWriter, r *http.Request) {
	_, stack, ok := rt.stackSyncTarget(w, r)
	if !ok {
		return
	}
	if rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "webhook secrets need a master key on this control plane")
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		rt.internalError(w, "api: stack sync webhook secret: random failed", err)
		return
	}
	secret := hex.EncodeToString(raw)
	if err := rt.gitSourceSecrets.SetValue(r.Context(), store.StackSyncSecretsKey(stack.ID), stackSyncWebhookSecretKey, secret); err != nil {
		rt.internalError(w, "api: stack sync webhook secret: store failed", err)
		return
	}
	rt.auditStack(r, auditStackSyncWebhookRotate, map[string]any{"stack": stack.Name})
	w.Header().Set("Cache-Control", "no-store")
	base := rt.syncHookOrigin(r) + "/api/v1/hooks/stacks/" + stack.Name
	writeJSON(w, http.StatusOK, stackSyncWebhookSecretOutput{Secret: secret, SyncURL: base + "/sync", RefreshURL: base + "/refresh"})
}

// syncHookOrigin is the origin a git host posts to: the configured dashboard URL, else
// this request's own scheme and Host. Forwarded headers are never trusted here.
func (rt *Router) syncHookOrigin(r *http.Request) string {
	if base := rt.dashboardURL(); base != "" {
		return strings.TrimRight(base, "/")
	}
	scheme := "https://"
	if host := strings.Split(r.Host, ":")[0]; host == "localhost" || host == "127.0.0.1" {
		scheme = "http://"
	}
	return scheme + r.Host
}

// handleStackSyncWebhook handles POST /api/v1/hooks/stacks/{name}/sync.
func (rt *Router) handleStackSyncWebhook(w http.ResponseWriter, r *http.Request) {
	rt.serveStackSyncWebhook(w, r, "sync")
}

// handleStackSyncRefreshWebhook handles POST /api/v1/hooks/stacks/{name}/refresh.
func (rt *Router) handleStackSyncRefreshWebhook(w http.ResponseWriter, r *http.Request) {
	rt.serveStackSyncWebhook(w, r, "refresh")
}

// serveStackSyncWebhook never logs the body or headers. Every refusal of
// identity (stack, sync off, secret) answers one 404 so names cannot be probed.
func (rt *Router) serveStackSyncWebhook(w http.ResponseWriter, r *http.Request, verb string) {
	name := r.PathValue("name")
	ip := clientIP(r)
	if rt.webhookIPRateLimit != nil {
		if allowed, retryAfter := rt.webhookIPRateLimit.allow(ip); !allowed {
			writeRateLimited(w, retryAfter)
			return
		}
	}
	if rt.webhookRateLimit != nil {
		if allowed, retryAfter := rt.webhookRateLimit.allow(ip + "|stack/" + name); !allowed {
			writeRateLimited(w, retryAfter)
			return
		}
	}
	maxBytes := int64(64) << 10
	if rt.runtimeSettings != nil {
		maxBytes = int64(rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyStackSyncWebhookMaxKB)) << 10
	}
	if r.ContentLength > maxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "content type must be application/json")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBytes))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "cannot read request body")
		return
	}

	rec, secret, found := rt.stackSyncWebhookTarget(r.Context(), name)
	provider, signed := verifyStackSyncWebhook(secret, found, body, r.Header)
	if !found || !signed {
		rt.logger.Warn("api: stack sync webhook refused", slog.String("remote_addr", ip))
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	if ignored := stackSyncIgnoredEvent(provider, r.Header); ignored != "" {
		writeJSON(w, http.StatusAccepted, map[string]string{"ignored": ignored})
		return
	}
	// Only the ref and the head commit are read; the rest of the payload is ignored.
	var push struct {
		Ref     string `json:"ref"`
		After   string `json:"after"`
		Deleted bool   `json:"deleted"`
	}
	if err := json.Unmarshal(body, &push); err != nil || push.Ref == "" {
		writeError(w, http.StatusBadRequest, "malformed push payload")
		return
	}
	branch, isBranch := strings.CutPrefix(push.Ref, "refs/heads/")
	switch {
	case strings.HasPrefix(push.Ref, "refs/tags/"):
		writeJSON(w, http.StatusAccepted, map[string]string{"ignored": "tag"})
		return
	case !isBranch:
		writeJSON(w, http.StatusAccepted, map[string]string{"ignored": "ref"})
		return
	case push.Deleted || push.After == "" || isZeroSHA(push.After):
		writeJSON(w, http.StatusAccepted, map[string]string{"ignored": "deleted"})
		return
	case !isCommitSHA(push.After):
		writeError(w, http.StatusBadRequest, "malformed push payload")
		return
	}
	want := rec.Config.Branch
	if want == "" {
		want = webhook.DefaultBranch
	}
	if branch != want {
		writeJSON(w, http.StatusAccepted, map[string]string{"ignored": "branch"})
		return
	}
	syncer := rt.stackSyncState().syncer
	if syncer == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	short := push.After[:12]
	outcome := syncer.Trigger(rec, verb == "sync")
	rt.logger.Info("stack sync webhook triggered", slog.String("stack", rec.StackName), slog.String("verb", verb),
		slog.String("commit", short), slog.String("provider", provider), slog.String("outcome", outcome))
	if rt.eventWebhooks != nil {
		rt.eventWebhooks.Emit(eventhook.Event{Type: eventStackSyncTriggered, Data: map[string]any{
			"stack": rec.StackName, "verb": verb, "branch": branch, "commit": short, "provider": provider, "outcome": outcome,
		}})
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": outcome, "verb": verb, "branch": branch, "commit": short})
}

// stackSyncWebhookTarget answers found false for every reason a delivery cannot
// be checked: no stack, no sync row, sync off, no secret, or no secret store.
func (rt *Router) stackSyncWebhookTarget(ctx context.Context, name string) (store.StackSyncRecord, string, bool) {
	ss, sy := rt.stackStore(), rt.stackSyncStore()
	if ss == nil || sy == nil || rt.gitSourceSecrets == nil {
		return store.StackSyncRecord{}, "", false
	}
	stack, err := ss.GetStackByName(ctx, name)
	if err != nil {
		return store.StackSyncRecord{}, "", false
	}
	rec, found, err := sy.GetStackSync(ctx, stack.ID)
	if err != nil || !found || !rec.Config.Enabled {
		return store.StackSyncRecord{}, "", false
	}
	secret, err := rt.gitSourceSecrets.Resolve(ctx, store.StackSyncSecretsKey(stack.ID), stackSyncWebhookSecretKey)
	if err != nil || secret == "" {
		return store.StackSyncRecord{}, "", false
	}
	return rec, secret, true
}

// verifyStackSyncWebhook checks a GitLab token or a GitHub SHA-256 signature.
// A delivery signed with SHA-1 alone fails. Both compares are constant time.
func verifyStackSyncWebhook(secret string, found bool, body []byte, h http.Header) (string, bool) {
	if !found {
		secret = stackSyncWebhookDecoy
	}
	if token := h.Get(webhook.HeaderGitLabToken); token != "" {
		// Hashing both sides first keeps the compare independent of the token length.
		got, wantSum := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(secret))
		return "gitlab", hmac.Equal(got[:], wantSum[:]) && found
	}
	return "github", webhook.VerifySignature([]byte(secret), body, h.Get(webhook.HeaderHubSignature256)) && found
}

// stackSyncIgnoredEvent names a signed delivery that is not a branch push.
func stackSyncIgnoredEvent(provider string, h http.Header) string {
	if provider == "gitlab" {
		switch h.Get(webhook.HeaderGitLabEvent) {
		case "Push Hook":
			return ""
		case "Tag Push Hook":
			return "tag"
		}
		return "event"
	}
	switch h.Get(webhook.HeaderGitHubEvent) {
	case "push":
		return ""
	case "ping":
		return "ping"
	}
	return "event"
}

func isCommitSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
