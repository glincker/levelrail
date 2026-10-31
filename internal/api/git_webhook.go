package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/sourcecache"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// Fetches repoURL at an exact commit sha into a directory, authenticated with token when set.
// The token is what makes private repos cloneable here.
type gitSourceFetchFunc func(ctx context.Context, repoURL, sha, token string) (dir string, cleanup func(), err error)

// Multi-app webhook handler for POST /api/v1/webhooks/github/{name}. Unauthenticated by design: GitHub and GitLab cannot present a session,
// so the per-app secret checked by verifyGitPushWebhookAuth stands in for auth. The URL still says github for already-configured webhooks.
// Returns 404 when no git source is connected for name; 501 is reserved for control-plane-wide gaps.
func (rt *Router) handleGitPushWebhook(w http.ResponseWriter, r *http.Request) {
	// Explicit text/plain stops clients from sniffing success bodies, which embed pusher-controlled fields, as HTML.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	name := r.PathValue("name")

	// Rate limit runs first, before the 501 and any DB or secret work, so abusive clients are cheap to reject.
	if rt.webhookRateLimit != nil {
		key := clientIP(r) + "|" + name
		if allowed, retryAfter := rt.webhookRateLimit.allow(key); !allowed {
			rt.logger.Warn("api: git push webhook: rate limited", slog.String("name", name), slog.String("remote_addr", r.RemoteAddr))
			writeRateLimited(w, retryAfter)
			return
		}
	}

	if rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "git sources are not configured on this control plane (no master key set)")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, webhook.MaxPayloadBytes+1))
	if err != nil {
		rt.logger.Warn("api: git push webhook: read request body failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusBadRequest, "cannot read request body")
		return
	}
	if len(body) > webhook.MaxPayloadBytes {
		rt.logger.Warn("api: git push webhook: request body exceeds max payload size", slog.String("name", name), slog.Int("max_bytes", webhook.MaxPayloadBytes))
		writeError(w, http.StatusBadRequest, "request body too large")
		return
	}

	provider, eventType, headerFields := detectWebhookProviderAndEvent(r.Header)

	gs, err := rt.gitSources.GetGitSource(r.Context(), name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		rt.saveWebhookDelivery(r.Context(), name, provider, eventType, headerFields, false, false, http.StatusNotFound, body, "no git source connected for this app", "")
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	}
	if err != nil {
		rt.internalError(w, "api: git push webhook: load git source failed", err, slog.String("name", name))
		return
	}

	secretsKey := store.GitSourceSecretsKey(name)
	secret, err := rt.gitSourceSecrets.Resolve(r.Context(), secretsKey, gitSourceSecretKey)
	if err != nil {
		rt.internalError(w, "api: git push webhook: resolve webhook secret failed", err, slog.String("name", name))
		return
	}

	if !verifyGitPushWebhookAuth(secret, body, r.Header) {
		rt.logger.Warn("api: git push webhook: signature verification failed", slog.String("name", name), slog.String("remote_addr", r.RemoteAddr))
		rt.saveWebhookDelivery(r.Context(), name, provider, eventType, headerFields, false, true, http.StatusUnauthorized, body, "invalid signature", "")
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	deliveryID := webhookDeliveryID(r.Header)
	if dup := rt.duplicateWebhookDelivery(r.Context(), name, deliveryID); dup != nil {
		rt.saveWebhookDeliveryIDs(r.Context(), name, provider, eventType, headerFields, true, true, dup.StatusCode, body, "", webhookNoteDuplicate, deliveryID, "")
		w.WriteHeader(dup.StatusCode)
		_, _ = fmt.Fprintf(w, "%s of %s\n", webhookNoteDuplicate, dup.ID)
		return
	}

	bodyKey := webhookBodyKey(provider+"|"+eventType, name, body)
	if rt.webhookReplayEnabled(r.Context()) && !rt.webhookBodies.claim(bodyKey, time.Now(), rt.webhookReplayWindow(r.Context())) {
		rt.logger.Warn("api: git push webhook: replayed body ignored", slog.String("name", name), slog.String("provider", provider))
		rt.saveWebhookDeliveryIDs(r.Context(), name, provider, eventType, headerFields, true, true, http.StatusOK, body, "", webhookNoteReplayedBody, deliveryID, "")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "%s\n", webhookNoteReplayedBody)
		return
	}

	pctx, outcome := withWebhookOutcome(r.Context())
	status, message := rt.processGitPushWebhookPayload(pctx, name, *gs, body, r.Header)
	// Only a delivery that started a deploy keeps its claim; rejected or ignored ones stay retryable.
	if status >= http.StatusBadRequest || outcome.AttemptID == "" {
		rt.webhookBodies.release(bodyKey)
	}
	deliveryErr, deliveryNote := "", ""
	if status >= http.StatusBadRequest {
		deliveryErr = message
	} else {
		deliveryNote = webhookDeliveryNote(message)
	}
	rt.saveWebhookDeliveryIDs(r.Context(), name, provider, eventType, headerFields, true, true, status, body, deliveryErr, deliveryNote, deliveryID, outcome.AttemptID)
	if outcome.AttemptID != "" {
		rt.kickDeployQueue(r.Context())
	}

	w.WriteHeader(status)
	if _, err := fmt.Fprint(w, message); err != nil { //nolint:gosec // gosec's taint check can't see the text/plain Content-Type set above this function's entry, which is the actual mitigation for a client ever interpreting message's webhook-payload-derived fields as markup
		rt.logger.Warn("api: git push webhook: failed to write response body", slog.String("error", err.Error()), slog.String("name", name))
	}
}

// Runs push and pull-request handling on an already signature-verified body.
// Shared with handleReplayWebhookDelivery so replays take the identical path.
func (rt *Router) processGitPushWebhookPayload(ctx context.Context, name string, gs store.GitSource, body []byte, header http.Header) (status int, message string) {
	if webhook.IsPullRequestEvent(header) {
		prEv, err := webhook.ParsePullRequestEventForProvider(body, header)
		if err != nil {
			rt.logger.Warn("api: git push webhook: malformed pull request payload", slog.String("error", err.Error()), slog.String("name", name))
			return http.StatusBadRequest, "malformed payload"
		}
		rt.firePipelinePullRequest(ctx, name, prEv)
		return rt.handlePullRequestWebhookEvent(ctx, name, gs, prEv)
	}

	if webhook.IsMergeGroupEvent(header) {
		mg, err := webhook.ParseMergeGroupEvent(body)
		if err != nil {
			rt.logger.Warn("api: git push webhook: malformed merge_group payload", slog.String("error", err.Error()), slog.String("name", name))
			return http.StatusBadRequest, "malformed payload"
		}
		rt.firePipelineMergeGroup(ctx, name, mg)
		return http.StatusOK, "merge_group event handled\n"
	}

	if isGitHubReleaseEvent(header) {
		return rt.processGitHubReleaseWebhookEvent(ctx, name, gs, body)
	}

	if isGitHubPingEvent(header) {
		return http.StatusOK, "ping event acknowledged\n"
	}

	if webhook.IsIssueCommentEvent(header) {
		return rt.handleIssueCommentWebhook(ctx, name, gs, body)
	}

	ev, err := webhook.ParsePushEventForProvider(body, header.Get(webhook.HeaderBitbucketEventKey))
	if err != nil {
		rt.logger.Warn("api: git push webhook: malformed payload", slog.String("error", err.Error()), slog.String("name", name))
		return http.StatusBadRequest, "malformed payload"
	}
	if ok, pushed := pushRepoMatches(ev, gs.RepoURL); !ok {
		rt.logger.Info("api: git push webhook: push is for another repository", slog.String("name", name), slog.String("pushed_repo", pushed))
		return http.StatusOK, fmt.Sprintf("ignored: push is for %s, this app tracks %s\n", pushed, repoIdentity(gs.RepoURL))
	}

	rt.firePipelinePushEvent(ctx, name, ev)
	ctx = withPushCommitMeta(ctx, ev)

	if status, msg, handled := rt.handleBranchDeployPush(ctx, name, gs, ev); handled {
		return status, msg
	}

	triggered, ignoredMsg := gitSourceTriggerMatchesPush(gs, ev.Ref)
	if !triggered {
		rt.logger.Info("api: git push webhook: ignoring push", slog.String("name", name), slog.String("ref", ev.Ref), slog.String("trigger_mode", effectiveGitSourceTriggerMode(gs.TriggerMode)))
		return http.StatusOK, ignoredMsg
	}
	if msg := skipPushForMarker(ev.HeadMessage); msg != "" && !isWebhookReplay(ctx) {
		rt.logger.Info("api: git push webhook: push skipped by commit marker", slog.String("name", name), slog.String("ref", ev.Ref))
		return http.StatusOK, msg
	}
	if msg := rt.skipPushForPaths(ctx, name, gs, ev.Ref, ev.Before, ev.After, ev.Changed); msg != "" {
		rt.logger.Info("api: git push webhook: push skipped by path filter", slog.String("name", name), slog.String("ref", ev.Ref))
		return http.StatusOK, msg
	}

	rt.noteGitHead(ctx, name, ev.After)
	if status, msg, handled := rt.admitGitPush(ctx, name, ev); handled {
		return status, msg
	}
	if msg := rt.gateOnCI(ctx, name, gs, ev.After); msg != "" {
		return http.StatusOK, msg
	}
	queued, msg, release := rt.queueIfBusy(ctx, name, gs, deploy.HeldRequest{Kind: deploy.HeldKindGit, CheckoutRef: ev.After, CommitLabel: ev.After, CommitSHA: ev.After, Before: ev.Before, CommitAt: ev.HeadCommitAt, Branch: strings.TrimPrefix(ev.Ref, "refs/heads/")}, name+":"+ev.After)
	defer release()
	if queued {
		return http.StatusAccepted, msg
	}
	return rt.deployFromGitSource(withStartRelease(ctx, release), name, gs, ev.After, ev.After, rt.nextOrder(ctx, name, ev.After, ev.Before, ev.HeadCommitAt))
}

// Routes GitHub's release event before ParsePushEventForProvider, since its payload is neither a push nor a pull_request.
func isGitHubReleaseEvent(header http.Header) bool {
	return header.Get(webhook.HeaderGitHubEvent) == "release"
}

// Routes GitHub's ping before ParsePushEventForProvider, or the first delivery of every new source shows a 400.
func isGitHubPingEvent(header http.Header) bool {
	return header.Get(webhook.HeaderGitHubEvent) == "ping"
}

// Push mode matches only the configured branch. Release mode matches only tag pushes, on any branch.
// GitHub release events are handled separately, before this function is reached.
func gitSourceTriggerMatchesPush(gs store.GitSource, ref string) (bool, string) {
	if effectiveGitSourceTriggerMode(gs.TriggerMode) == spec.TriggerModeRelease {
		if strings.HasPrefix(ref, "refs/tags/") {
			return true, ""
		}
		return false, fmt.Sprintf("ignored: push to %q, trigger mode is %q which only deploys on a tag push or a published github release\n", ref, spec.TriggerModeRelease)
	}
	targetCfg := webhook.Config{Branch: gs.Branch}
	if ref == targetCfg.TargetRef() {
		return true, ""
	}
	return false, fmt.Sprintf("ignored: push to %q, deploys trigger on %q only\n", ref, targetCfg.TargetRef())
}

// githubReleaseEvent is the subset of GitHub's release webhook payload
// this package needs: https://docs.github.com/en/webhooks/webhook-events-and-payloads#release.
type githubReleaseEvent struct {
	Action  string
	TagName string
}

// parseGitHubReleaseEvent decodes body as a GitHub release event
// payload, requiring a non-empty action (tag_name is validated by the
// caller, since it's only required once action is actually "published").
func parseGitHubReleaseEvent(body []byte) (githubReleaseEvent, error) {
	var payload struct {
		Action  string `json:"action"`
		Release struct {
			TagName string `json:"tag_name"`
		} `json:"release"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return githubReleaseEvent{}, fmt.Errorf("api: malformed github release payload: %w", err)
	}
	if payload.Action == "" {
		return githubReleaseEvent{}, errors.New("api: github release payload missing action")
	}
	return githubReleaseEvent{Action: payload.Action, TagName: payload.Release.TagName}, nil
}

// Deploys only on a published release, not a draft or other action.
// The tag name is sanitized for Docker image tags, since git tags may contain '/'.
func (rt *Router) processGitHubReleaseWebhookEvent(ctx context.Context, name string, gs store.GitSource, body []byte) (status int, message string) {
	if effectiveGitSourceTriggerMode(gs.TriggerMode) != spec.TriggerModeRelease {
		return http.StatusOK, "ignored: a github release event only deploys when this git source's trigger_mode is \"release\"\n"
	}

	rel, err := parseGitHubReleaseEvent(body)
	if err != nil {
		rt.logger.Warn("api: git push webhook: malformed release payload", slog.String("error", err.Error()), slog.String("name", name))
		return http.StatusBadRequest, "malformed payload"
	}
	if rel.Action != "published" {
		return http.StatusOK, fmt.Sprintf("ignored: release action %q, deploys trigger on \"published\" only\n", rel.Action)
	}
	if rel.TagName == "" {
		return http.StatusBadRequest, "malformed payload"
	}

	label := dockerSafeTag(rel.TagName)
	if held, msg := rt.holdIfFrozen(ctx, name, deploy.HeldRequest{Kind: deploy.HeldKindGit, CheckoutRef: "refs/tags/" + rel.TagName, CommitLabel: label}, name+":"+label); held {
		return http.StatusAccepted, msg
	}
	tagReq := deploy.HeldRequest{Kind: deploy.HeldKindGit, CheckoutRef: "refs/tags/" + rel.TagName, CommitLabel: label}
	if handled, status, hmsg := rt.holdIfProtected(ctx, name, tagReq, name+":"+label, store.DeployAttemptSourceWebhook, SystemApprovalActor); handled {
		return status, hmsg
	}
	queued, msg, release := rt.queueIfBusy(ctx, name, gs, deploy.HeldRequest{Kind: deploy.HeldKindGit, CheckoutRef: "refs/tags/" + rel.TagName, CommitLabel: label}, name+":"+label)
	defer release()
	if queued {
		return http.StatusAccepted, msg
	}
	return rt.deployFromGitSource(withStartRelease(ctx, release), name, gs, "refs/tags/"+rel.TagName, label, rt.nextOrder(ctx, name, "", "", time.Time{}))
}

// dockerSafeTag makes tagName safe to use as a Docker image tag
// (ImageRepo + ":" + tag, internal/deploy.Request.CommitSHA): a git tag
// may contain "/", which Docker's own tag grammar rejects.
func dockerSafeTag(tagName string) string {
	return strings.ReplaceAll(tagName, "/", "-")
}

// Shared by push and release paths so both use the identical services and AdditionalServices routing.
func (rt *Router) deployFromGitSource(ctx context.Context, name string, gs store.GitSource, checkoutRef, commitLabel string, order *store.DeployOrder) (status int, message string) {
	return rt.deployFromGitSourceAs(ctx, name, gs, checkoutRef, commitLabel, order, store.DeployAttemptSourceWebhook)
}

// Sets the deploy attempt source, so scheduled redeploys are distinguishable from git pushes in deploy history.
func (rt *Router) deployFromGitSourceAs(ctx context.Context, name string, gs store.GitSource, checkoutRef, commitLabel string, order *store.DeployOrder, source string) (status int, message string) {
	dep := rt.beginForgeDeployment(ctx, name, gs, checkoutRef, forgeEnvProduction, forgeEnvProduction, "")
	status, message = rt.deployFromGitSourceInner(ctx, name, gs, checkoutRef, commitLabel, order, source)
	dep.finish(ctx, deploymentStateFor(status, message), strings.TrimSpace(message))
	return status, message
}

func (rt *Router) deployFromGitSourceInner(ctx context.Context, name string, gs store.GitSource, checkoutRef, commitLabel string, order *store.DeployOrder, source string) (status int, message string) {
	if rt.builder == nil {
		return http.StatusNotImplemented, "git push deploys are not configured on this control plane"
	}

	existing, err := rt.apps.GetDesiredService(ctx, name)
	if errors.Is(err, store.ErrServiceNotFound) {
		return http.StatusNotFound, "app not found"
	}
	if err != nil {
		rt.logger.Error("api: git push webhook: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		return http.StatusInternalServerError, "internal error"
	}

	token, err := rt.resolveGitSourceDeployToken(ctx, name)
	if err != nil {
		rt.logger.Error("api: git push webhook: resolve deploy token failed", slog.String("error", err.Error()), slog.String("name", name))
		return http.StatusInternalServerError, "internal error"
	}
	sourceDir, cleanup, err := rt.gitSourceFetch(ctx, gs.RepoURL, checkoutRef, token)
	if err != nil {
		rt.logger.Error("api: git push webhook: fetch source failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("checkout_ref", checkoutRef))
		if _, adopted := adoptedAttemptID(ctx); errors.Is(err, errGitCommitNotFound) && !adopted {
			return http.StatusUnprocessableEntity, "commit not found"
		}
		// Record a failed attempt even when the fetch fails, so the Deploys page shows the real cause.
		// The response stays generic.
		_, _, finishFetchAttempt, _ := rt.beginBuildDeployAttempt(ctx, deploy.Request{
			ServiceName: name,
			CommitSHA:   commitLabel,
			ImageRepo:   defaultImageRepo(name, existing.Image),
		}, *existing, source, "")
		finishFetchAttempt(fmt.Errorf("fetch source: %w", err))
		if errors.Is(err, errGitCommitNotFound) {
			return http.StatusUnprocessableEntity, "commit not found"
		}
		if errors.Is(err, sourcecache.ErrHostUnreachable) {
			rt.noteGitHostBlocked(gs.RepoURL, name, err)
			return http.StatusServiceUnavailable, sourcecache.ErrHostUnreachable.Error()
		}
		return http.StatusInternalServerError, "deploy failed"
	}
	defer cleanup()

	// Services fan out via DeploySpec. validateGitSourceServices makes them mutually exclusive with AdditionalServices, so this is the one decision point.
	if len(gs.Services) > 0 {
		appName := existing.AppID
		if appName == "" {
			appName = name
		}
		failed := rt.deployServicesSpecFanout(ctx, appName, gs, sourceDir, commitLabel)
		status := http.StatusOK
		if failed > 0 {
			status = http.StatusMultiStatus
		}
		return status, fmt.Sprintf("services deploy triggered for app %q (%d service(s) failed)\n", appName, failed)
	}

	gitBuild, err := gs.SpecBuild()
	if err != nil {
		rt.logger.Error("api: git push webhook: invalid build settings", slog.String("error", err.Error()), slog.String("name", name))
		return http.StatusUnprocessableEntity, "invalid build settings: " + err.Error()
	}
	svcSpec := specServiceFromDesired(*existing, gitBuild)
	svcSpec.Build.Args = mergeBuildArgs(svcSpec.Build.Args, extraBuildArgsFrom(ctx))

	buildReq := deploy.Request{
		ServiceName: name,
		Service:     svcSpec,
		SourceDir:   sourceDir,
		CommitSHA:   commitLabel,
		ImageRepo:   defaultImageRepo(name, existing.Image),
		Order:       order,
	}

	attemptID, progress, finishAttempt, _ := rt.beginBuildDeployAttempt(ctx, buildReq, *existing, source, "")
	buildReq.AttemptID = attemptID
	buildCtx := ctx
	if attemptID != "" {
		buildReq.Commit = func() error { return rt.cancels.Commit(attemptID) }
		buildCtx = rt.cancels.Bind(ctx, attemptID)
	}
	tag, err := rt.builder.Deploy(buildCtx, buildReq, progress)
	_, canceled := rt.cancels.Canceled(attemptID)
	finishAttempt(err)
	if canceled && err != nil {
		return http.StatusOK, "not deployed: deploy canceled\n"
	}
	if errors.Is(err, deploy.ErrSuperseded) {
		rt.logger.Info("api: git push webhook: deploy superseded", slog.String("name", name), slog.String("commit", commitLabel), slog.String("reason", err.Error()))
		return http.StatusOK, "not deployed: " + err.Error() + "\n"
	}
	if err != nil {
		rt.logger.Error("api: git push webhook: deploy failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("commit", commitLabel))
		return http.StatusInternalServerError, "deploy failed"
	}
	rt.logger.Info("api: git push webhook: deploy triggered", slog.String("name", name), slog.String("commit", commitLabel), slog.String("tag", tag))

	additionalFailed := rt.deployAdditionalServices(ctx, gs.AdditionalServices, sourceDir, commitLabel)

	status = http.StatusOK
	if additionalFailed > 0 {
		status = http.StatusMultiStatus
	}
	return status, fmt.Sprintf("deploy triggered: %s (%d additional service(s) failed)\n", tag, additionalFailed)
}

// Identifies provider and event from headers, for delivery records only.
// GHE sends the same headers as github.com, so both report as github.
func detectWebhookProviderAndEvent(header http.Header) (provider, eventType string, headerFields map[string]string) {
	headerFields = map[string]string{
		webhook.HeaderGitHubEvent:       header.Get(webhook.HeaderGitHubEvent),
		webhook.HeaderGitLabEvent:       header.Get(webhook.HeaderGitLabEvent),
		webhook.HeaderBitbucketEventKey: header.Get(webhook.HeaderBitbucketEventKey),
		webhook.HeaderGiteaEventType:    header.Get(webhook.HeaderGiteaEventType),
	}
	switch {
	case headerFields[webhook.HeaderGitLabEvent] != "":
		return "gitlab", headerFields[webhook.HeaderGitLabEvent], headerFields
	case headerFields[webhook.HeaderGiteaEventType] != "":
		// Checked before X-GitHub-Event: Gitea also sends X-Hub-Signature-256 but never X-GitHub-Event, so this cannot shadow a GitHub delivery.
		if header.Get("X-Forgejo-Event") != "" {
			return "forgejo", headerFields[webhook.HeaderGiteaEventType], headerFields
		}
		return "gitea", headerFields[webhook.HeaderGiteaEventType], headerFields
	case headerFields[webhook.HeaderGitHubEvent] != "":
		return "github", headerFields[webhook.HeaderGitHubEvent], headerFields
	case headerFields[webhook.HeaderBitbucketEventKey] != "":
		return "bitbucket", headerFields[webhook.HeaderBitbucketEventKey], headerFields
	default:
		return "unknown", "unknown", headerFields
	}
}

// Logs and swallows store errors: history tracking must never fail the webhook response.
func (rt *Router) saveWebhookDelivery(ctx context.Context, name, provider, eventType string, headerFields map[string]string, signatureValid, matched bool, statusCode int, payload []byte, errMsg, note string) {
	rt.saveWebhookDeliveryIDs(ctx, name, provider, eventType, headerFields, signatureValid, matched, statusCode, payload, errMsg, note, "", "")
}

func (rt *Router) saveWebhookDeliveryIDs(ctx context.Context, name, provider, eventType string, headerFields map[string]string, signatureValid, matched bool, statusCode int, payload []byte, errMsg, note, deliveryID, attemptID string) {
	id, err := store.NewWebhookDeliveryID()
	if err != nil {
		rt.logger.Error("api: save webhook delivery: mint id failed", slog.String("error", err.Error()), slog.String("name", name))
		return
	}
	truncated := false
	if !signatureValid {
		if limit := rt.failedWebhookBodyBytes(); len(payload) > limit {
			payload, truncated = payload[:limit], true
		}
	}
	if err := rt.webhookDeliveries.SaveWebhookDelivery(ctx, store.WebhookDelivery{
		ID: id, ServiceName: name, Provider: provider, EventType: eventType,
		HeaderFields: headerFields, SignatureValid: signatureValid, Matched: matched,
		StatusCode: statusCode, Payload: payload, PayloadTruncated: truncated, Error: errMsg, Note: note, ReceivedAt: time.Now(),
		DeliveryID: deliveryID, AttemptID: attemptID,
	}); err != nil {
		rt.logger.Error("api: save webhook delivery failed", slog.String("error", err.Error()), slog.String("name", name))
		return
	}
	if !signatureValid {
		if _, err := rt.webhookDeliveries.PruneFailedWebhookDeliveries(ctx, name, rt.failedWebhookKeep()); err != nil {
			rt.logger.Error("api: prune failed webhook deliveries failed", slog.String("error", err.Error()), slog.String("name", name))
		}
	}
}

const (
	defaultWebhookFailedKeep      = 20
	defaultWebhookFailedBodyBytes = 2048
)

func (rt *Router) failedWebhookKeep() int {
	if rt.webhookFailedKeep > 0 {
		return rt.webhookFailedKeep
	}
	return defaultWebhookFailedKeep
}

func (rt *Router) failedWebhookBodyBytes() int {
	if rt.webhookFailedBodyBytes > 0 {
		return rt.webhookFailedBodyBytes
	}
	return defaultWebhookFailedBodyBytes
}

// maxWebhookDeliveryNoteBytes caps a stored outcome note: the message can
// echo a payload-derived ref or branch name, so it is bounded like any
// other value copied out of an inbound request.
const maxWebhookDeliveryNoteBytes = 300

// webhookDeliveryNote turns a handler outcome message into the single
// trimmed line stored on the delivery row.
func webhookDeliveryNote(message string) string {
	note := strings.TrimSpace(message)
	if len(note) > maxWebhookDeliveryNoteBytes {
		note = note[:maxWebhookDeliveryNoteBytes]
	}
	return note
}

// Returns the connected source's deploy token, or empty for public repos.
// Shared with preview deploys, so private-repo credentials resolve identically.
func (rt *Router) resolveGitSourceDeployToken(ctx context.Context, name string) (string, error) {
	secretsKey := store.GitSourceSecretsKey(name)
	hasToken, err := rt.gitSourceSecrets.Exists(ctx, secretsKey, gitSourceTokenKey)
	if err != nil {
		return "", fmt.Errorf("check deploy token: %w", err)
	}
	if hasToken {
		token, err := rt.gitSourceSecrets.Resolve(ctx, secretsKey, gitSourceTokenKey)
		if err != nil {
			return "", fmt.Errorf("resolve deploy token: %w", err)
		}
		return token, nil
	}
	// No deploy token: fall back to the connected GitHub App's installation token, so private repos work without a stored PAT.
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if err != nil || gs == nil {
		return "", nil
	}
	return rt.tokenForRepo(ctx, name, gs.RepoURL, true), nil
}

// Redeploys each sibling from the same checkout. One sibling's failure does not block the others; returns the failure count.
func (rt *Router) deployAdditionalServices(ctx context.Context, additional map[string]store.GitSourceBuild, sourceDir, commitSHA string) int {
	failed := 0
	for svcName, buildCfg := range additional {
		svc, err := rt.apps.GetDesiredService(ctx, svcName)
		if err != nil {
			rt.logger.Error("api: git push webhook: additional service: load failed", slog.String("service", svcName), slog.String("error", err.Error()))
			failed++
			continue
		}

		svcSpec := specServiceFromDesired(*svc, spec.Build{Type: buildCfg.BuildType, Path: buildCfg.BuildPath})
		req := deploy.Request{
			ServiceName: svcName,
			Service:     svcSpec,
			SourceDir:   sourceDir,
			CommitSHA:   commitSHA,
			ImageRepo:   svcName,
		}

		_, progress, finishAttempt, _ := rt.beginBuildDeployAttempt(ctx, req, *svc, store.DeployAttemptSourceWebhook, "")
		tag, err := rt.builder.Deploy(ctx, req, progress)
		finishAttempt(err)
		if err != nil {
			rt.logger.Error("api: git push webhook: additional service: deploy failed", slog.String("service", svcName), slog.String("error", err.Error()))
			failed++
			continue
		}
		rt.logger.Info("api: git push webhook: additional service: deploy triggered", slog.String("service", svcName), slog.String("tag", tag))
	}
	return failed
}

// Routes a webhook push through the same DeploySpec fan-out as the manual deploy-spec API.
// appName matches DeploySpec's default so a push reuses the existing app group. Returns the failed key count.
func (rt *Router) deployServicesSpecFanout(ctx context.Context, appName string, gs store.GitSource, sourceDir, commitSHA string) int {
	progress := func(serviceKey string, ev build.ProgressEvent) {
		rt.logger.Info("api: git push webhook: services fan-out build progress", slog.String("app_name", appName), slog.String("service_key", serviceKey), slog.String("step", ev.Step), slog.Bool("completed", ev.Completed), slog.String("error", ev.Error))
	}

	outcomes, err := rt.builder.DeploySpec(ctx, deploy.MultiRequest{
		AppName:       appName,
		Services:      gs.Services,
		SourceDir:     sourceDir,
		CommitSHA:     commitSHA,
		ImageRepoBase: appName,
	}, progress)
	if err != nil {
		rt.logger.Error("api: git push webhook: services fan-out failed", slog.String("app_name", appName), slog.String("error", err.Error()))
		return len(gs.Services)
	}

	failed := 0
	for _, o := range outcomes {
		if o.Err != nil {
			failed++
			rt.logger.Error("api: git push webhook: services fan-out: service failed", slog.String("app_name", appName), slog.String("service_key", o.ServiceKey), slog.String("error", o.Err.Error()))
			continue
		}
		rt.logger.Info("api: git push webhook: services fan-out: service deployed", slog.String("app_name", appName), slog.String("service_key", o.ServiceKey), slog.String("image", o.Image))
	}
	return failed
}

// Checks a push webhook against secret using the scheme the headers indicate: GitHub and Bitbucket sign the body (HMAC),
// GitLab sends the secret verbatim (compared in constant time). Gitea uses the GitHub branch. Missing or mismatched headers fail closed.
func verifyGitPushWebhookAuth(secret string, body []byte, header http.Header) bool {
	if token := header.Get(webhook.HeaderGitLabToken); token != "" {
		return secret != "" && subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1
	}
	if sig := header.Get(webhook.HeaderHubSignature256); sig != "" {
		return webhook.VerifySignature([]byte(secret), body, sig)
	}
	return webhook.VerifySignature([]byte(secret), body, header.Get(webhook.HeaderHubSignature))
}
