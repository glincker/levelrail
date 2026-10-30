package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultPipelineWebhookMaxBody = 64 * 1024
	defaultPipelineWebhookKeep    = 100
	pipelineHookPrefix            = "/api/v1/hooks/pipeline/"
	pipelineHMACSecretKey         = "hmac_secret"
	pipelineSignatureHeader       = "X-Hub-Signature-256"
	maxIdempotencyKeyLen          = 128
	defaultPipelineMaxRefLen      = 255
)

var (
	pipelineRefRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/@+-]*$`)
	pipelineSHARe = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)
)

// validatePipelineRefSHA keeps hostile ref and sha values out of the OIDC
// subject, the checkout script and commit statuses. Empty values are allowed.
func validatePipelineRefSHA(ref, sha string) error {
	maxLen := envPositiveInt("LEVELRAIL_PIPELINE_MAX_REF_LEN", defaultPipelineMaxRefLen)
	if ref != "" {
		if len(ref) > maxLen || !pipelineRefRe.MatchString(ref) || strings.Contains(ref, "..") ||
			strings.Contains(ref, "//") || strings.HasSuffix(ref, "/") || strings.HasSuffix(ref, ".") ||
			strings.HasSuffix(ref, ".lock") || strings.Contains(ref, "@{") {
			return fmt.Errorf("%w: ref is not a valid git ref", pipeline.ErrInvalidInput)
		}
	}
	if sha != "" && !pipelineSHARe.MatchString(sha) {
		return fmt.Errorf("%w: sha must be 7 to 64 hex characters", pipeline.ErrInvalidInput)
	}
	return nil
}

// pipelineHookContentTypes are the payload types a trigger accepts. The
// body is stored and exposed as data, never parsed into commands.
var pipelineHookContentTypes = map[string]bool{
	"application/json":                  true,
	"application/x-www-form-urlencoded": true,
	"text/plain":                        true,
}

type pipelineWebhookResource struct {
	Configured bool       `json:"configured"`
	Enabled    bool       `json:"enabled,omitempty"`
	HMAC       bool       `json:"hmac,omitempty"`
	Path       string     `json:"path,omitempty"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
	RotatedAt  *time.Time `json:"rotated_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	// Token and Secret appear once, when minted or rotated. Only their
	// hash (token) or the secrets store (secret) keeps them afterwards.
	Token  string `json:"token,omitempty"`
	Secret string `json:"secret,omitempty"`
}

type pipelineWebhookDeliveryResource struct {
	ID             string    `json:"id"`
	ReceivedAt     time.Time `json:"received_at"`
	StatusCode     int       `json:"status_code"`
	Outcome        string    `json:"outcome"`
	Reason         string    `json:"reason,omitempty"`
	RunID          string    `json:"run_id,omitempty"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	ContentType    string    `json:"content_type,omitempty"`
	BodyBytes      int       `json:"body_bytes"`
	Replayable     bool      `json:"replayable"`
}

func toWebhookResource(pipelineID string, t store.PipelineWebhookTrigger) pipelineWebhookResource {
	return pipelineWebhookResource{
		Configured: true, Enabled: t.Enabled, HMAC: t.HMAC, Path: pipelineHookPrefix + pipelineID + "/{token}",
		CreatedAt: &t.CreatedAt, RotatedAt: &t.RotatedAt, LastUsedAt: t.LastUsedAt,
	}
}

func (rt *Router) loadWebhookPipeline(w http.ResponseWriter, r *http.Request) (store.Pipeline, bool) {
	if !rt.extrasReady(w) {
		return store.Pipeline{}, false
	}
	return rt.loadPipeline(w, r)
}

// handleGetPipelineWebhook handles GET .../pipelines/{pname}/webhook.
func (rt *Router) handleGetPipelineWebhook(w http.ResponseWriter, r *http.Request) {
	p, ok := rt.loadWebhookPipeline(w, r)
	if !ok {
		return
	}
	t, err := rt.pipelineExtras.GetPipelineWebhookTrigger(r.Context(), p.ID)
	if errors.Is(err, store.ErrPipelineWebhookNotFound) {
		writeJSON(w, http.StatusOK, pipelineWebhookResource{})
		return
	}
	if err != nil {
		rt.internalError(w, "api: load pipeline webhook failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toWebhookResource(p.ID, t))
}

type pipelineWebhookRequest struct {
	HMAC    *bool `json:"hmac"`
	Enabled *bool `json:"enabled"`
}

// handleSetPipelineWebhook handles PUT .../pipelines/{pname}/webhook: it
// creates the trigger (returning the token once) or updates its flags.
func (rt *Router) handleSetPipelineWebhook(w http.ResponseWriter, r *http.Request) {
	p, ok := rt.loadWebhookPipeline(w, r)
	if !ok {
		return
	}
	var req pipelineWebhookRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, pipelineBodyLimit)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	now := time.Now().UTC()
	t, err := rt.pipelineExtras.GetPipelineWebhookTrigger(r.Context(), p.ID)
	fresh := errors.Is(err, store.ErrPipelineWebhookNotFound)
	if err != nil && !fresh {
		rt.internalError(w, "api: load pipeline webhook failed", err)
		return
	}
	wasHMAC := t.HMAC
	if fresh {
		t = store.PipelineWebhookTrigger{PipelineID: p.ID, Enabled: true, CreatedAt: now, RotatedAt: now}
	}
	if req.Enabled != nil {
		t.Enabled = *req.Enabled
	}
	if req.HMAC != nil {
		t.HMAC = *req.HMAC
	}
	var token, secret string
	if fresh {
		if token, err = randomToken(); err != nil {
			rt.internalError(w, "api: mint pipeline webhook token", err)
			return
		}
		t.TokenHash = hashToken(token)
	}
	if t.HMAC && (fresh || !wasHMAC) {
		if secret, ok = rt.mintPipelineHMACSecret(w, r, p.ID); !ok {
			return
		}
	}
	if err := rt.pipelineExtras.SavePipelineWebhookTrigger(r.Context(), t); err != nil {
		rt.internalError(w, "api: save pipeline webhook failed", err)
		return
	}
	res := toWebhookResource(p.ID, t)
	if token != "" {
		res.Token, res.Path = token, pipelineHookPrefix+p.ID+"/"+token
	}
	res.Secret = secret
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) mintPipelineHMACSecret(w http.ResponseWriter, r *http.Request, pipelineID string) (string, bool) {
	if rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "signed webhooks need secret storage, which is not configured (no master key set)")
		return "", false
	}
	secret, err := randomToken()
	if err == nil {
		err = rt.gitSourceSecrets.SetValue(r.Context(), store.PipelineWebhookSecretsKey(pipelineID), pipelineHMACSecretKey, secret)
	}
	if err != nil {
		rt.internalError(w, "api: store pipeline webhook secret", err)
		return "", false
	}
	return secret, true
}

// handleRotatePipelineWebhook handles POST .../webhook/rotate.
func (rt *Router) handleRotatePipelineWebhook(w http.ResponseWriter, r *http.Request) {
	p, ok := rt.loadWebhookPipeline(w, r)
	if !ok {
		return
	}
	t, err := rt.pipelineExtras.GetPipelineWebhookTrigger(r.Context(), p.ID)
	if errors.Is(err, store.ErrPipelineWebhookNotFound) {
		writeError(w, http.StatusNotFound, "this pipeline has no webhook trigger")
		return
	}
	if err != nil {
		rt.internalError(w, "api: load pipeline webhook failed", err)
		return
	}
	token, err := randomToken()
	if err != nil {
		rt.internalError(w, "api: mint pipeline webhook token", err)
		return
	}
	t.TokenHash, t.RotatedAt = hashToken(token), time.Now().UTC()
	var secret string
	if t.HMAC {
		if secret, ok = rt.mintPipelineHMACSecret(w, r, p.ID); !ok {
			return
		}
	}
	if err := rt.pipelineExtras.SavePipelineWebhookTrigger(r.Context(), t); err != nil {
		rt.internalError(w, "api: rotate pipeline webhook failed", err)
		return
	}
	res := toWebhookResource(p.ID, t)
	res.Token, res.Path, res.Secret = token, pipelineHookPrefix+p.ID+"/"+token, secret
	writeJSON(w, http.StatusOK, res)
}

// handleDeletePipelineWebhook handles DELETE .../webhook.
func (rt *Router) handleDeletePipelineWebhook(w http.ResponseWriter, r *http.Request) {
	p, ok := rt.loadWebhookPipeline(w, r)
	if !ok {
		return
	}
	if err := rt.pipelineExtras.DeletePipelineWebhookTrigger(r.Context(), p.ID); err != nil {
		rt.internalError(w, "api: delete pipeline webhook failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListPipelineWebhookDeliveries handles GET .../webhook/deliveries.
// Bodies are not returned; replay re-reads them server side.
func (rt *Router) handleListPipelineWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	p, ok := rt.loadWebhookPipeline(w, r)
	if !ok {
		return
	}
	rows, err := rt.pipelineExtras.ListPipelineWebhookDeliveries(r.Context(), p.ID, 50)
	if err != nil {
		rt.internalError(w, "api: list pipeline webhook deliveries failed", err)
		return
	}
	out := make([]pipelineWebhookDeliveryResource, 0, len(rows))
	for _, d := range rows {
		out = append(out, pipelineWebhookDeliveryResource{
			ID: d.ID, ReceivedAt: d.ReceivedAt, StatusCode: d.StatusCode, Outcome: d.Outcome, Reason: d.Reason, RunID: d.RunID,
			IdempotencyKey: d.IdempotencyKey, ContentType: d.ContentType, BodyBytes: len(d.Body), Replayable: replayable(d),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func replayable(d store.PipelineWebhookDelivery) bool {
	return d.Outcome == store.WebhookOutcomeStarted || d.Outcome == store.WebhookOutcomeFailed
}

// handleReplayPipelineWebhookDelivery handles POST .../webhook/deliveries/{did}/replay.
func (rt *Router) handleReplayPipelineWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	if !rt.extrasReady(w) || !rt.runnerReady(w) {
		return
	}
	p, ok := rt.loadPipeline(w, r)
	if !ok {
		return
	}
	d, err := rt.pipelineExtras.GetPipelineWebhookDelivery(r.Context(), p.ID, r.PathValue("did"))
	if errors.Is(err, store.ErrPipelineDeliveryNotFound) {
		writeError(w, http.StatusNotFound, "delivery not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: load pipeline webhook delivery failed", err)
		return
	}
	if !replayable(d) {
		writeError(w, http.StatusConflict, "only accepted deliveries can be replayed")
		return
	}
	if !p.Enabled {
		writeError(w, http.StatusConflict, "pipeline is disabled")
		return
	}
	run, err := rt.startWebhookRun(r, p, d.Body, d.ContentType, d.Query, "replay:"+rt.pipelineActor(r))
	if errors.Is(err, pipeline.ErrInvalidInput) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	nd := store.PipelineWebhookDelivery{
		ID: newPipelineID(), PipelineID: p.ID, ReceivedAt: time.Now().UTC(), StatusCode: http.StatusAccepted,
		Outcome: store.WebhookOutcomeStarted, Reason: "replay of " + d.ID, RunID: run.ID, ContentType: d.ContentType, Query: d.Query, Body: d.Body,
	}
	if _, err := rt.pipelineExtras.AddPipelineWebhookDelivery(r.Context(), nd); err != nil {
		rt.logger.Warn("api: record pipeline webhook replay failed", "error", err.Error(), "pipeline_id", p.ID)
	}
	writeJSON(w, http.StatusAccepted, toRunResource(run, p.Name))
}

// startWebhookRun starts a webhook-triggered run. Query values become
// run parameters (checked against the declared ones); ref and sha are reserved.
func (rt *Router) startWebhookRun(r *http.Request, p store.Pipeline, body []byte, contentType, rawQuery, actor string) (store.PipelineRun, error) {
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return store.PipelineRun{}, fmt.Errorf("%w: unreadable query string", pipeline.ErrInvalidInput)
	}
	if err := validatePipelineRefSHA(q.Get("ref"), q.Get("sha")); err != nil {
		return store.PipelineRun{}, err
	}
	inputs := map[string]string{}
	for k, v := range q {
		if k != "ref" && k != "sha" && len(v) > 0 {
			inputs[k] = v[0]
		}
	}
	return rt.pipelineRunner.Start(r.Context(), p, pipeline.StartOptions{
		Trigger: pipeline.TriggerWebhook, Actor: actor, Ref: q.Get("ref"), SHA: q.Get("sha"),
		Inputs: inputs, Payload: body, PayloadContentType: contentType,
	})
}

// handlePipelineWebhook handles POST /api/v1/hooks/pipeline/{id}/{token}.
// Unauthenticated by session: the URL token (compared by hash) and, in
// signed mode, the body HMAC are the credentials. An unknown pipeline, a
// wrong token, and a disabled trigger all answer the same 404.
func (rt *Router) handlePipelineWebhook(w http.ResponseWriter, r *http.Request) {
	id, token := r.PathValue("id"), r.PathValue("token")
	if rt.pipelineExtras == nil || rt.pipelineRunner == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if rt.webhookRateLimit != nil {
		if allowed, retryAfter := rt.webhookRateLimit.allow(clientIP(r) + "|pipeline-hook|" + id); !allowed {
			writeRateLimited(w, retryAfter)
			return
		}
	}
	t, err := rt.pipelineExtras.GetPipelineWebhookTrigger(r.Context(), id)
	hash := t.TokenHash
	if err != nil {
		hash = ""
	}
	if subtle.ConstantTimeCompare([]byte(hashToken(token)), []byte(hash)) != 1 || !t.Enabled {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	d := store.PipelineWebhookDelivery{ID: newPipelineID(), PipelineID: id, ReceivedAt: time.Now().UTC(), Query: r.URL.RawQuery}
	reject := func(status int, reason string) {
		d.StatusCode, d.Outcome, d.Reason = status, store.WebhookOutcomeRejected, reason
		rt.recordPipelineDelivery(r, d)
		writeError(w, status, reason)
	}
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if !pipelineHookContentTypes[ct] {
		reject(http.StatusUnsupportedMediaType, "content type must be application/json, application/x-www-form-urlencoded, or text/plain")
		return
	}
	d.ContentType = ct
	maxBody := rt.pipelineWebhookMaxBody
	if maxBody <= 0 {
		maxBody = defaultPipelineWebhookMaxBody
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		reject(http.StatusRequestEntityTooLarge, fmt.Sprintf("body is larger than %d bytes", maxBody))
		return
	}
	if t.HMAC {
		if msg := rt.verifyPipelineSignature(r, id, body); msg != "" {
			reject(http.StatusUnauthorized, msg)
			return
		}
	}
	if ct == "application/json" && !json.Valid(body) {
		reject(http.StatusBadRequest, "body is not valid JSON")
		return
	}
	key := strings.TrimSpace(firstHeader(r, "Idempotency-Key", "X-GitHub-Delivery"))
	if len(key) > maxIdempotencyKeyLen {
		reject(http.StatusBadRequest, "idempotency key is too long")
		return
	}
	d.IdempotencyKey, d.Outcome, d.Body = key, store.WebhookOutcomeReceived, body
	rt.acceptPipelineDelivery(w, r, d)
}

func firstHeader(r *http.Request, names ...string) string {
	for _, n := range names {
		if v := r.Header.Get(n); v != "" {
			return v
		}
	}
	return ""
}

// verifyPipelineSignature checks the GitHub-style sha256 HMAC. It returns
// an empty string on success and the reason otherwise. Any failure to read
// the secret fails closed.
func (rt *Router) verifyPipelineSignature(r *http.Request, pipelineID string, body []byte) string {
	got, ok := strings.CutPrefix(r.Header.Get(pipelineSignatureHeader), "sha256=")
	if !ok || got == "" {
		return "missing " + pipelineSignatureHeader + " header"
	}
	if rt.gitSourceSecrets == nil {
		return "signature cannot be checked: secret storage is not configured"
	}
	secret, err := rt.gitSourceSecrets.Resolve(r.Context(), store.PipelineWebhookSecretsKey(pipelineID), pipelineHMACSecretKey)
	if err != nil || secret == "" {
		rt.logger.Error("api: pipeline webhook secret unavailable", "pipeline_id", pipelineID)
		return "signature cannot be checked: shared secret unavailable"
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.ToLower(got)), []byte(want)) {
		return "signature does not match"
	}
	return ""
}

func (rt *Router) acceptPipelineDelivery(w http.ResponseWriter, r *http.Request, d store.PipelineWebhookDelivery) {
	ctx := r.Context()
	first, err := rt.pipelineExtras.AddPipelineWebhookDelivery(ctx, d)
	if errors.Is(err, store.ErrPipelineDeliveryDuplicate) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "duplicate", "run_id": first.RunID, "delivery_id": first.ID})
		return
	}
	if err != nil {
		rt.internalError(w, "api: record pipeline webhook delivery failed", err)
		return
	}
	finish := func(status int, outcome, reason, runID string) {
		if err := rt.pipelineExtras.FinishPipelineWebhookDelivery(ctx, d.ID, outcome, reason, runID, status); err != nil {
			rt.logger.Warn("api: finish pipeline webhook delivery failed", "error", err.Error())
		}
		_ = rt.pipelineExtras.TouchPipelineWebhookTrigger(ctx, d.PipelineID, time.Now().UTC())
		rt.prunePipelineDeliveries(r, d.PipelineID)
	}
	p, err := rt.pipelineExtras.GetPipeline(ctx, d.PipelineID)
	if err != nil {
		finish(http.StatusInternalServerError, store.WebhookOutcomeFailed, "pipeline could not be loaded", "")
		rt.internalError(w, "api: load pipeline for webhook failed", err)
		return
	}
	if def, issues := pipeline.Validate([]byte(p.YAML)); !p.Enabled || len(issues) > 0 || def.On.Webhook == nil {
		reason := "pipeline does not accept webhook triggers (add on.webhook to the file)"
		if !p.Enabled {
			reason = "pipeline is disabled"
		}
		finish(http.StatusConflict, store.WebhookOutcomeFailed, reason, "")
		writeError(w, http.StatusConflict, reason)
		return
	}
	run, err := rt.startWebhookRun(r, p, d.Body, d.ContentType, d.Query, "webhook")
	if err != nil {
		finish(http.StatusBadRequest, store.WebhookOutcomeFailed, err.Error(), "")
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	finish(http.StatusAccepted, store.WebhookOutcomeStarted, "", run.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "started", "run_id": run.ID, "number": run.Number, "delivery_id": d.ID})
}

func (rt *Router) recordPipelineDelivery(r *http.Request, d store.PipelineWebhookDelivery) {
	d.Body = []byte{}
	if _, err := rt.pipelineExtras.AddPipelineWebhookDelivery(r.Context(), d); err != nil {
		rt.logger.Warn("api: record rejected pipeline webhook failed", "error", err.Error(), "pipeline_id", d.PipelineID)
		return
	}
	rt.prunePipelineDeliveries(r, d.PipelineID)
}

func (rt *Router) prunePipelineDeliveries(r *http.Request, pipelineID string) {
	keep := rt.pipelineWebhookKeep
	if keep <= 0 {
		keep = defaultPipelineWebhookKeep
	}
	if err := rt.pipelineExtras.PrunePipelineWebhookDeliveries(r.Context(), pipelineID, keep); err != nil {
		rt.logger.Warn("api: prune pipeline webhook deliveries failed", "error", err.Error(), "pipeline_id", pipelineID)
	}
}
