package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ImageAutoUpdateStore is the store surface for per-app image auto-update.
type ImageAutoUpdateStore interface {
	SetImageAutoUpdate(ctx context.Context, serviceName string, enabled bool) error
	GetImageAutoUpdate(ctx context.Context, serviceName string) (store.ImageAutoUpdate, error)
	RecordImageAutoUpdateCheck(ctx context.Context, serviceName, result string, at time.Time) error
	SetImageAutoUpdateWindow(ctx context.Context, serviceName, cron string, minutes int) error
	SetImageAutoUpdateWebhookHash(ctx context.Context, serviceName, hash string) error
	DeleteImageAutoUpdatesForApp(ctx context.Context, serviceName string) error
}

// Results CheckImageUpdate records; the UI and CLI show them verbatim.
const (
	imageUpdateUpToDate   = "up to date"
	imageUpdateRedeployed = "redeployed with a newer image"
	imageUpdateWaiting    = "newer image found, waiting for the update window"
)

type imageAutoUpdateResource struct {
	Enabled       bool       `json:"enabled"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	LastResult    string     `json:"last_result,omitempty"`
	HasWebhook    bool       `json:"has_webhook"`
	// HasSigningSecret reports whether GitHub package deliveries must carry a valid X-Hub-Signature-256.
	HasSigningSecret bool `json:"has_signing_secret"`
	// WindowCron and WindowMinutes: updates found outside the window wait for it.
	WindowCron    string `json:"window_cron,omitempty"`
	WindowMinutes int    `json:"window_minutes,omitempty"`
}

type setImageAutoUpdateRequest struct {
	Enabled bool `json:"enabled"`
	// Window fields are left untouched when omitted; an empty cron clears the window.
	WindowCron    *string `json:"window_cron,omitempty"`
	WindowMinutes *int    `json:"window_minutes,omitempty"`
}

func toImageAutoUpdateResource(u store.ImageAutoUpdate) imageAutoUpdateResource {
	return imageAutoUpdateResource{Enabled: u.Enabled, LastCheckedAt: u.LastCheckedAt, LastResult: u.LastResult, HasWebhook: u.WebhookHash != "", WindowCron: u.WindowCron, WindowMinutes: u.WindowMinutes}
}

func (rt *Router) handleGetImageAutoUpdate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: get image auto-update: load app", err, slog.String("name", name))
		return
	}
	u, err := rt.imageAutoUpdates.GetImageAutoUpdate(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get image auto-update", err, slog.String("name", name))
		return
	}
	res := toImageAutoUpdateResource(u)
	res.HasSigningSecret = rt.hasPushSigningSecret(r.Context(), name)
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) handleSetImageAutoUpdate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req setImageAutoUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set image auto-update: load app", err, slog.String("name", name))
		return
	}
	cron, minutes, werr := resolveUpdateWindow(req)
	if werr != nil {
		writeError(w, http.StatusBadRequest, werr.Error())
		return
	}
	if req.WindowCron != nil {
		if err := rt.imageAutoUpdates.SetImageAutoUpdateWindow(r.Context(), name, cron, minutes); err != nil {
			rt.internalError(w, "api: set image auto-update window", err, slog.String("name", name))
			return
		}
	}
	if err := rt.imageAutoUpdates.SetImageAutoUpdate(r.Context(), name, req.Enabled); err != nil {
		rt.internalError(w, "api: set image auto-update", err, slog.String("name", name))
		return
	}
	u, err := rt.imageAutoUpdates.GetImageAutoUpdate(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: set image auto-update: reload", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, toImageAutoUpdateResource(u))
}

// handleCheckImageAutoUpdate runs one update check now, regardless of the opt-in.
func (rt *Router) handleCheckImageAutoUpdate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	result, err := rt.checkImageUpdateRecorded(r.Context(), name, true)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Warn("api: image update check failed", slog.String("name", name), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	u, err := rt.imageAutoUpdates.GetImageAutoUpdate(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: image update check: reload", err, slog.String("name", name))
		return
	}
	res := toImageAutoUpdateResource(u)
	res.LastResult = result
	writeJSON(w, http.StatusOK, res)
}

// CheckImageUpdate compares serviceName's tag against its registry and
// redeploys when the digest moved. Skips (with a reason, not an error) apps
// built from source, stopped, frozen, or in a protected environment: an
// unattended pull must never bypass the gates a manual deploy goes through.
func (rt *Router) CheckImageUpdate(ctx context.Context, serviceName string) (string, error) {
	return rt.checkImageUpdateRecorded(ctx, serviceName, false)
}

// checkImageUpdateRecorded is CheckImageUpdate; manual checks ignore the apply window.
func (rt *Router) checkImageUpdateRecorded(ctx context.Context, serviceName string, manual bool) (string, error) {
	result, err := rt.checkImageUpdate(ctx, serviceName, manual)
	if err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			return "", err
		}
		rt.logger.Warn("api: image update check failed", slog.String("name", serviceName), slog.String("error", err.Error()))
		result = "check failed: the image could not be checked against its registry"
	}
	if recErr := rt.imageAutoUpdates.RecordImageAutoUpdateCheck(ctx, serviceName, result, time.Now()); recErr != nil {
		rt.logger.Warn("api: record image update check failed", slog.String("name", serviceName), slog.String("error", recErr.Error()))
	}
	return result, err
}

func (rt *Router) checkImageUpdate(ctx context.Context, serviceName string, manual bool) (string, error) {
	svc, err := rt.apps.GetDesiredService(ctx, serviceName)
	if err != nil {
		return "", err
	}
	if skip := rt.imageUpdateSkipReason(ctx, *svc); skip != "" {
		return "skipped: " + skip, nil
	}
	if rt.imageResolver == nil {
		return "", errors.New("no image resolver is configured on this control plane")
	}
	tagRef := docker.UnpinImageRef(svc.Image)
	trigger := rt.imageTrigger(ctx, *svc, true, "image auto-update")
	remote, err := rt.imageResolver.ResolveImage(ctx, tagRef, trigger.Auth, true)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", tagRef, err)
	}
	if remote.Digest == "" || remote.Digest == docker.ImageDigestOf(svc.Image) {
		return imageUpdateUpToDate, nil
	}
	if !manual {
		if u, uerr := rt.imageAutoUpdates.GetImageAutoUpdate(ctx, serviceName); uerr == nil && !inUpdateWindow(u.WindowCron, u.WindowMinutes, time.Now()) {
			return imageUpdateWaiting, nil
		}
	}
	trigger.Source = store.DeployAttemptSourceImage
	trigger.Locks = rt.deployPublishLocks()
	trigger.Ordered, trigger.Sequencer = nil, nil
	updated, err := trigger.Deploy(ctx, *svc, tagRef)
	if err != nil {
		return "", err
	}
	if rt.deployNotifier != nil {
		rt.deployNotifier.Dispatch(ctx, resourceIDForApp(svc.Name), alerting.DeployOutcome{AppName: svc.Name, Image: updated.Image, Succeeded: true})
	}
	return imageUpdateRedeployed, nil
}

func (rt *Router) imageUpdateSkipReason(ctx context.Context, svc store.DesiredService) string {
	switch {
	case svc.ImageIDRef != "":
		return "built from source, redeploy from git instead"
	case svc.Suspended:
		return "app is stopped"
	case rt.hasRunningDeployAttempt(ctx, svc.Name):
		return "a deploy is already running"
	}
	if frozen, reason := rt.freezeStatus(ctx, svc.Name); frozen {
		return "deploy freeze active: " + reason
	}
	if svc.EnvironmentID != "" {
		env, err := rt.environments.GetEnvironment(ctx, svc.EnvironmentID)
		switch {
		case errors.Is(err, store.ErrEnvironmentNotFound):
		case err != nil:
			return "could not check whether the environment is protected"
		case env.Protected:
			return "protected environment, deploy it manually"
		}
	}
	return ""
}

const imageUpdateWebhookTimeout = 2 * time.Minute

type imageUpdateWebhookResource struct {
	Path  string `json:"path"`
	Token string `json:"token"`
	// SigningSecret is the GitHub package webhook secret, shown once alongside the token; empty when secrets are not configured.
	SigningSecret string `json:"signing_secret,omitempty"`
}

func (rt *Router) hasPushSigningSecret(ctx context.Context, name string) bool {
	if rt.gitSourceSecrets == nil {
		return false
	}
	has, err := rt.gitSourceSecrets.Exists(ctx, imageUpdateSecretNS+name, imageUpdateSecretKey)
	return err == nil && has
}

// handleRotateImageUpdateWebhook mints a fresh webhook token, shown once.
func (rt *Router) handleRotateImageUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: rotate image update webhook: load app", err, slog.String("name", name))
		return
	}
	// One rotation per app at a time, so the recorded rotator is always the person whose URL and secret are live.
	mu := rt.pushRotateLock(name)
	mu.Lock()
	defer mu.Unlock()
	token, err := randomToken()
	if err != nil {
		rt.internalError(w, "api: rotate image update webhook: mint token", err)
		return
	}
	res := imageUpdateWebhookResource{Path: imageUpdateHookPrefix + name + "/" + token, Token: token}
	// The new secret is staged (accepted beside the old one, if any) and the rotator recorded before the new URL is
	// saved. If anything up to the save fails, both are put back, so the live hook keeps its secret and its rotator.
	// After the save the caller always gets the URL and secret, and a failed promotion is finished by the next signed
	// delivery or rotation.
	var secret string
	if rt.gitSourceSecrets != nil {
		prev := rt.currentPushRotator(r.Context(), name)
		var serr error
		if secret, serr = randomToken(); serr == nil {
			serr = rt.gitSourceSecrets.SetValue(r.Context(), imageUpdateSecretNS+name, imageUpdateSecretPendingKey, secret)
		}
		if serr == nil {
			serr = rt.recordPushRotator(r, name)
		}
		if serr != nil {
			rt.rollbackStagedRotation(r.Context(), name, prev)
			rt.internalError(w, "api: rotate image update webhook: stage signing secret", serr, slog.String("name", name))
			return
		}
		if err := rt.imageAutoUpdates.SetImageAutoUpdateWebhookHash(r.Context(), name, hashToken(token)); err != nil {
			rt.rollbackStagedRotation(r.Context(), name, prev)
			rt.internalError(w, "api: rotate image update webhook", err, slog.String("name", name))
			return
		}
		rt.promotePushSecret(r.Context(), name, secret)
		res.SigningSecret = secret
		writeJSON(w, http.StatusOK, res)
		return
	}
	if err := rt.imageAutoUpdates.SetImageAutoUpdateWebhookHash(r.Context(), name, hashToken(token)); err != nil {
		rt.internalError(w, "api: rotate image update webhook", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// currentPushRotator is the recorded rotator, or the none sentinel when there is none to restore.
func (rt *Router) currentPushRotator(ctx context.Context, name string) string {
	has, err := rt.gitSourceSecrets.Exists(ctx, imageUpdateSecretNS+name, imageUpdateRotatorKey)
	if err != nil || !has {
		return pushRotatorNone
	}
	human, err := rt.gitSourceSecrets.Resolve(ctx, imageUpdateSecretNS+name, imageUpdateRotatorKey)
	if err != nil || human == "" {
		return pushRotatorNone
	}
	return human
}

// rollbackStagedRotation empties the staged secret and restores the previous rotator after a failed rotation. A restore
// that itself fails leaves the rotator as the none sentinel when possible, so approvals fail closed.
func (rt *Router) rollbackStagedRotation(ctx context.Context, name, prevRotator string) {
	ctx = context.WithoutCancel(ctx)
	if err := rt.gitSourceSecrets.SetValue(ctx, imageUpdateSecretNS+name, imageUpdateSecretPendingKey, pushPendingNone); err != nil {
		rt.logger.Warn("api: rollback staged signing secret failed", slog.String("name", name), slog.String("error", err.Error()))
	}
	if err := rt.gitSourceSecrets.SetValue(ctx, imageUpdateSecretNS+name, imageUpdateRotatorKey, prevRotator); err != nil {
		rt.logger.Warn("api: restore webhook rotator failed", slog.String("name", name), slog.String("error", err.Error()))
		_ = rt.gitSourceSecrets.SetValue(ctx, imageUpdateSecretNS+name, imageUpdateRotatorKey, pushRotatorNone)
	}
}

// pushRotateLock's map holds one small entry per app that was ever rotated; it is never pruned.
func (rt *Router) pushRotateLock(name string) *sync.Mutex {
	mu, _ := rt.registryPushRotateLocks.LoadOrStore(name, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// handleImageUpdateWebhook is the public registry push hook. Unknown app,
// wrong token and opted-out all answer the same 404 so the URL leaks nothing.
func (rt *Router) handleImageUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	name, token := r.PathValue("name"), r.PathValue("token")
	if ok, retry := rt.buildHookLimits().failures.allow("image-probe:" + clientIP(r)); !ok {
		tooManyBuildHookRequests(w, retry)
		return
	}
	u, err := rt.imageAutoUpdates.GetImageAutoUpdate(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: image update webhook", err, slog.String("name", name))
		return
	}
	if !u.Enabled || u.WebhookHash == "" || subtle.ConstantTimeCompare([]byte(hashToken(token)), []byte(u.WebhookHash)) != 1 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if rt.handleRegistryPushDelivery(w, r, name, u) {
		return
	}
	if u.LastCheckedAt != nil && time.Since(*u.LastCheckedAt) < rt.runtimeSettings.Duration(r.Context(), runtimesettings.KeyDeploysRegistryPushDebounce) {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "checked recently"})
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), imageUpdateWebhookTimeout)
		defer cancel()
		if _, err := rt.CheckImageUpdate(ctx, name); err != nil {
			rt.logger.Warn("api: image update webhook check failed", slog.String("name", name), slog.String("error", err.Error()))
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "check started"})
}
