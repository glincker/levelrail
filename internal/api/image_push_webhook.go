package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/distribution/reference"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Registry providers whose push events the image-update hook understands.
const (
	registryProviderGHCR      = "ghcr"
	registryProviderDockerHub = "dockerhub"
)

const (
	imageUpdateSecretNS         = "image-update-webhook/"
	imageUpdateSecretKey        = "signing_secret"
	imageUpdateSecretPendingKey = "signing_secret_pending"
	imageUpdateRotatorKey       = "rotated_by"
	// pushRotatorNone marks a rotation by a caller with no person behind it, so no approval may be filed.
	pushRotatorNone = "none"
	// pushPendingNone marks the staged secret slot as empty: the store has no delete, and a random secret never equals it.
	pushPendingNone    = "none"
	headerGitHubEvent  = "X-GitHub-Event"
	headerGitHubSig256 = "X-Hub-Signature-256"
	// ActorTypeRegistryPush labels deploys a registry push started, in audit rows and approvals.
	actorTypeRegistryPush = "registry_push"
	// AuditActionRegistryPushDeploy marks the outcome of one registry push delivery that passed authentication.
	AuditActionRegistryPushDeploy = "registry_push.deploy"
	// AuditActionRegistryPushSupersede marks a pending approval a newer push rejected.
	AuditActionRegistryPushSupersede = "registry_push.supersede"
)

var pushDigestRE = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// registryPush is one verified-shape push event: which repository and tag moved, and to which digest when the provider says.
type registryPush struct {
	Provider string
	// Repo is the normalized repository name, such as ghcr.io/acme/web.
	Repo   string
	Tag    string
	Digest string
	// PayloadDigest is the digest the delivery claimed when it disagreed with the registry.
	PayloadDigest string
	// signed is true when a valid GitHub signature vouched for this delivery.
	signed bool
	// RemoteAddr and UserAgent describe this delivery, so a queued run is audited as the delivery that queued it.
	RemoteAddr string
	UserAgent  string
	// hookHash is the webhook URL hash the delivery arrived on; a run refuses when it is no longer the live one.
	hookHash string
}

type registryPushActorKey struct{}

type registryPushActor struct{ provider, app string }

// withRegistryPushActor makes currentActor resolve to the push principal, so the shared deploy gates attribute the request.
func withRegistryPushActor(ctx context.Context, provider, app string) context.Context {
	return context.WithValue(ctx, registryPushActorKey{}, registryPushActor{provider: provider, app: app})
}

func registryPushActorFrom(ctx context.Context) (registryPushActor, bool) {
	a, ok := ctx.Value(registryPushActorKey{}).(registryPushActor)
	return a, ok
}

type ghcrPackage struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	PackageType string `json:"package_type"`
	Owner       struct {
		Login string `json:"login"`
	} `json:"owner"`
	PackageVersion struct {
		Version           string `json:"version"`
		ContainerMetadata struct {
			Tag struct {
				Name   string `json:"name"`
				Digest string `json:"digest"`
			} `json:"tag"`
		} `json:"container_metadata"`
	} `json:"package_version"`
}

type ghcrEnvelope struct {
	Action          string       `json:"action"`
	Package         *ghcrPackage `json:"package"`
	RegistryPackage *ghcrPackage `json:"registry_package"`
}

// parseGHCRPush reads a GitHub package or registry_package event. ignore is a human reason when the event is valid but not a tag publish.
func parseGHCRPush(body []byte) (push registryPush, ignore string, err error) {
	var env ghcrEnvelope
	if err = json.Unmarshal(body, &env); err != nil {
		return registryPush{}, "", errors.New("invalid json")
	}
	pkg := env.Package
	if pkg == nil {
		pkg = env.RegistryPackage
	}
	if pkg == nil {
		return registryPush{}, "", errors.New("no package in payload")
	}
	if env.Action != "published" && env.Action != "updated" {
		return registryPush{}, "event " + env.Action + " is not a publish", nil
	}
	if pt := strings.ToLower(pkg.PackageType); pt != "" && pt != "container" {
		return registryPush{}, "package is not a container image", nil
	}
	owner := pkg.Namespace
	if owner == "" {
		owner = pkg.Owner.Login
	}
	tag := pkg.PackageVersion.ContainerMetadata.Tag
	digest := tag.Digest
	if digest == "" && strings.HasPrefix(pkg.PackageVersion.Version, "sha256:") {
		digest = pkg.PackageVersion.Version
	}
	if owner == "" || pkg.Name == "" {
		return registryPush{}, "", errors.New("package owner or name missing")
	}
	if tag.Name == "" {
		return registryPush{}, "untagged publish", nil
	}
	if !pushDigestRE.MatchString(digest) {
		return registryPush{}, "", errors.New("package digest missing or malformed")
	}
	repo := "ghcr.io/" + strings.ToLower(owner) + "/" + strings.ToLower(pkg.Name)
	return registryPush{Provider: registryProviderGHCR, Repo: repo, Tag: tag.Name, Digest: digest}, "", nil
}

type dockerHubEnvelope struct {
	PushData struct {
		Tag string `json:"tag"`
	} `json:"push_data"`
	Repository struct {
		RepoName string `json:"repo_name"`
	} `json:"repository"`
}

// parseDockerHubPush reads a Docker Hub push webhook. The payload carries no digest, so Digest stays empty for the caller to resolve.
func parseDockerHubPush(body []byte) (registryPush, bool) {
	var env dockerHubEnvelope
	if json.Unmarshal(body, &env) != nil || env.PushData.Tag == "" || env.Repository.RepoName == "" {
		return registryPush{}, false
	}
	named, err := reference.ParseNormalizedNamed(env.Repository.RepoName)
	if err != nil || reference.Domain(named) != "docker.io" {
		return registryPush{}, false
	}
	return registryPush{Provider: registryProviderDockerHub, Repo: named.Name(), Tag: env.PushData.Tag}, true
}

// splitImageRef returns the repository as the app spells it, its normalized name and its tag (latest when none).
func splitImageRef(image string) (spelled, normalized, tag string, err error) {
	unpinned := docker.UnpinImageRef(image)
	named, err := reference.ParseNormalizedNamed(unpinned)
	if err != nil {
		return "", "", "", err
	}
	tag = "latest"
	spelled = unpinned
	if tagged, ok := named.(reference.Tagged); ok {
		tag = tagged.Tag()
		if i := strings.LastIndex(unpinned, ":"); i > strings.LastIndex(unpinned, "/") {
			spelled = unpinned[:i]
		}
	}
	return spelled, named.Name(), tag, nil
}

// verifyGitHubSignature checks X-Hub-Signature-256 over the raw body.
func verifyGitHubSignature(secret string, header string, body []byte) bool {
	hexSig, ok := strings.CutPrefix(strings.TrimSpace(header), "sha256=")
	if !ok {
		return false
	}
	got, err := hex.DecodeString(hexSig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// handleRegistryPushDelivery serves a body-carrying delivery on the image-update hook. handled=false means the request is not a
// recognized push event and the caller falls back to the plain "check the tag" behavior.
func (rt *Router) handleRegistryPushDelivery(w http.ResponseWriter, r *http.Request, name string, u store.ImageAutoUpdate) (handled bool) {
	githubEvent := r.Header.Get(headerGitHubEvent)
	if r.ContentLength == 0 && githubEvent == "" {
		return false
	}
	limit := int64(rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyDeploysRegistryPushMaxBody))
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		if err == nil {
			writeError(w, http.StatusRequestEntityTooLarge, "payload too large")
			return true
		}
		return false
	}
	var push registryPush
	switch {
	case githubEvent != "":
		ok, signed := rt.verifyPushSignature(w, r, name, body)
		if !ok {
			return true
		}
		push.signed = signed
		if githubEvent == "ping" {
			writeJSON(w, http.StatusOK, map[string]string{"status": "pong"})
			return true
		}
		if githubEvent != "package" && githubEvent != "registry_package" {
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "ignored: event " + githubEvent})
			return true
		}
		p, ignore, perr := parseGHCRPush(body)
		p.signed = push.signed
		if perr != nil {
			writeError(w, http.StatusBadRequest, "unrecognized package payload: "+perr.Error())
			return true
		}
		if ignore != "" {
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "ignored: " + ignore})
			return true
		}
		push = p
	default:
		p, ok := parseDockerHubPush(body)
		if !ok {
			return false
		}
		push = p
	}
	push.RemoteAddr, push.UserAgent, push.hookHash = clientIP(r), r.Header.Get("User-Agent"), u.WebhookHash
	rt.startRegistryPush(w, r, name, u, push)
	return true
}

// verifyPushSignature enforces the optional GitHub HMAC. With no secret set the URL token is the only gate.
// A secret staged by a rotation that has not finished is accepted too.
func (rt *Router) verifyPushSignature(w http.ResponseWriter, r *http.Request, name string, body []byte) (ok, signed bool) {
	if rt.gitSourceSecrets == nil {
		return true, false
	}
	var secrets []string
	hasMain := false
	for _, key := range []string{imageUpdateSecretKey, imageUpdateSecretPendingKey} {
		has, err := rt.gitSourceSecrets.Exists(r.Context(), imageUpdateSecretNS+name, key)
		if err != nil {
			rt.internalError(w, "api: image update webhook: check signing secret", err, slog.String("name", name))
			return false, false
		}
		if !has {
			continue
		}
		secret, err := rt.gitSourceSecrets.Resolve(r.Context(), imageUpdateSecretNS+name, key)
		if err != nil {
			rt.internalError(w, "api: image update webhook: resolve signing secret", err, slog.String("name", name))
			return false, false
		}
		if key == imageUpdateSecretPendingKey && secret == pushPendingNone {
			continue
		}
		secrets = append(secrets, secret)
		hasMain = hasMain || key == imageUpdateSecretKey
	}
	if len(secrets) == 0 {
		return true, false
	}
	for i, secret := range secrets {
		if verifyGitHubSignature(secret, r.Header.Get(headerGitHubSig256), body) {
			if i > 0 || (len(secrets) == 1 && !hasMain) {
				rt.promoteStagedSecret(r.Context(), name, secret)
			}
			return true, true
		}
	}
	rt.logger.Warn("api: image update webhook: bad signature", slog.String("name", name), slog.String("remote_addr", clientIP(r)))
	writeError(w, http.StatusNotFound, "not found")
	return false, false
}

// pushSlot serializes push deploys per app. A delivery that arrives while one runs is kept as next and runs after it,
// so the latest push is never lost; each run resolves the registry afresh, so a coalesced run is idempotent. The
// queue lives in memory: a restart drops it, and a deploy that outlives its deadline keeps the slot busy.
type pushSlot struct {
	mu      sync.Mutex
	running bool
	next    *registryPush
}

// registryPushSlot's map holds one small entry per app that ever received a push; it is never pruned.
// queue keeps push as the next run; the caller holds mu. An unsigned delivery never displaces a queued signed one:
// the run resolves the registry afresh either way.
func (s *pushSlot) queue(push registryPush) {
	if s.next == nil || push.signed || !s.next.signed {
		s.next = &push
	}
}

func (rt *Router) registryPushSlot(name string) *pushSlot {
	slot, _ := rt.registryPushLocks.LoadOrStore(name, &pushSlot{})
	return slot.(*pushSlot)
}

// startRegistryPush checks the push belongs to this app, then deploys its digest in the background.
func (rt *Router) startRegistryPush(w http.ResponseWriter, r *http.Request, name string, u store.ImageAutoUpdate, push registryPush) {
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: registry push: load app", err, slog.String("name", name))
		return
	}
	_, normalized, currentTag, err := splitImageRef(svc.Image)
	if err != nil || normalized != push.Repo {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	reason := rt.imagePushIgnoreReason(*svc, push, currentTag)
	if reason == "" {
		reason = rt.imagePushWindowReason(r.Context(), *svc, u)
	}
	if reason == "" && push.signed && push.Digest != "" && rt.recentlyHandled(r.Context(), u, push.Digest) {
		reason = "checked recently"
	}
	if reason != "" {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "ignored: " + reason})
		return
	}
	slot := rt.registryPushSlot(name)
	slot.mu.Lock()
	if slot.running {
		slot.queue(push)
		slot.mu.Unlock()
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued: runs after the push in progress"})
		return
	}
	slot.running = true
	slot.mu.Unlock()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "deploy started"})
	timeout := rt.runtimeSettings.Duration(r.Context(), runtimesettings.KeyDeploysRegistryPushTimeout)
	base := withRegistryPushActor(context.WithoutCancel(r.Context()), push.Provider, name)
	req := r.WithContext(base)
	go func() {
		for {
			rt.waitOutPushDebounce(base, name, push)
			ctx, cancel := context.WithTimeout(base, timeout)
			rt.runRegistryPush(req.WithContext(ctx), name, push)
			cancel()
			slot.mu.Lock()
			if slot.next == nil {
				slot.running = false
				slot.mu.Unlock()
				return
			}
			push, slot.next = *slot.next, nil
			slot.mu.Unlock()
		}
	}()
}

// waitOutPushDebounce spaces unsigned runs by the debounce, so each one that costs a registry lookup is rate limited
// without dropping the delivery.
func (rt *Router) waitOutPushDebounce(ctx context.Context, name string, push registryPush) {
	if push.signed {
		return
	}
	u, err := rt.imageAutoUpdates.GetImageAutoUpdate(ctx, name)
	if err != nil || u.LastCheckedAt == nil {
		return
	}
	// A last check in the future (a clock step back or a restored backup) counts as just now, so the wait never exceeds one debounce.
	debounce := rt.runtimeSettings.Duration(ctx, runtimesettings.KeyDeploysRegistryPushDebounce)
	wait := debounce - max(time.Since(*u.LastCheckedAt), 0)
	if wait <= 0 {
		return
	}
	select {
	case <-time.After(wait):
	case <-ctx.Done():
	}
}

// recentlyHandled is true when a signed delivery repeats the digest the last result already names.
func (rt *Router) recentlyHandled(ctx context.Context, u store.ImageAutoUpdate, digest string) bool {
	if u.LastCheckedAt == nil || time.Since(*u.LastCheckedAt) >= rt.runtimeSettings.Duration(ctx, runtimesettings.KeyDeploysRegistryPushDebounce) {
		return false
	}
	return strings.Contains(u.LastResult, shortDigest(digest))
}

// imagePushWindowReason applies the auto-update apply window. A protected environment skips it because a person
// approves anyway.
func (rt *Router) imagePushWindowReason(ctx context.Context, svc store.DesiredService, u store.ImageAutoUpdate) string {
	if rt.environmentProtected(ctx, svc.EnvironmentID) || inUpdateWindow(u.WindowCron, u.WindowMinutes, time.Now()) {
		return ""
	}
	if recErr := rt.imageAutoUpdates.RecordImageAutoUpdateCheck(ctx, svc.Name, imageUpdateWaiting, time.Now()); recErr != nil {
		rt.logger.Warn("api: record registry push wait failed", slog.String("name", svc.Name), slog.String("error", recErr.Error()))
	}
	return "outside the update window"
}

func (rt *Router) environmentProtected(ctx context.Context, environmentID string) bool {
	if environmentID == "" {
		return false
	}
	env, err := rt.environments.GetEnvironment(ctx, environmentID)
	return err == nil && env.Protected
}

func (rt *Router) imagePushIgnoreReason(svc store.DesiredService, push registryPush, currentTag string) string {
	switch {
	case svc.ImageIDRef != "":
		return "built from source, redeploy from git instead"
	case svc.Suspended:
		return "app is stopped"
	case push.Tag != currentTag:
		return "pushed tag is not the one this app follows"
	}
	return ""
}

// runRegistryPush resolves the digest, then runs the same gates as a manual image deploy. A protected environment
// files an approval for the digest, never deploys silently.
func (rt *Router) runRegistryPush(r *http.Request, name string, push registryPush) {
	ctx := r.Context()
	outcome, status := rt.runRegistryPushOnce(r, name, &push)
	if recErr := rt.imageAutoUpdates.RecordImageAutoUpdateCheck(ctx, name, outcome, time.Now()); recErr != nil {
		rt.logger.Warn("api: record registry push result failed", slog.String("name", name), slog.String("error", recErr.Error()))
	}
	rt.auditRegistryPush(r, name, push, outcome, status)
}

// runRegistryPushOnce reloads the app, so a coalesced run sees the state the previous one left.
func (rt *Router) runRegistryPushOnce(r *http.Request, name string, push *registryPush) (string, int) {
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if err != nil {
		return "push not deployed: could not load the app", http.StatusServiceUnavailable
	}
	spelled, normalized, currentTag, err := splitImageRef(svc.Image)
	if err != nil || normalized != push.Repo {
		return "push not deployed: the app no longer uses this repository", http.StatusConflict
	}
	// A queued run starts later than the delivery, so the app may have changed tag, stopped or left its window since.
	if reason := rt.imagePushIgnoreReason(*svc, *push, currentTag); reason != "" {
		return "push not deployed: " + reason, http.StatusConflict
	}
	u, err := rt.imageAutoUpdates.GetImageAutoUpdate(r.Context(), name)
	if err != nil {
		return "push not deployed: could not check the update window", http.StatusServiceUnavailable
	}
	// A run can start long after its delivery; opting out or rotating the URL in between must stop it.
	if !u.Enabled || u.WebhookHash != push.hookHash {
		return "push not deployed: auto-update was turned off or the webhook was rotated since the delivery", http.StatusConflict
	}
	if reason := rt.imagePushWindowReason(r.Context(), *svc, u); reason != "" {
		return "push not deployed: " + reason, http.StatusConflict
	}
	return rt.deployPushedDigest(r, *svc, spelled, push)
}

func (rt *Router) deployPushedDigest(r *http.Request, svc store.DesiredService, spelledRepo string, push *registryPush) (string, int) {
	ctx := r.Context()
	// The registry is the truth: a payload digest is only a claim, and a replayed or forged one must not pick the image.
	resolved, err := rt.resolvePushedDigest(ctx, svc, spelledRepo, push.Tag)
	if err != nil {
		return "push check failed: " + err.Error(), http.StatusBadGateway
	}
	if push.Digest != "" && push.Digest != resolved {
		push.PayloadDigest = push.Digest
		push.Digest = resolved
		return "push ignored: its digest is not what the registry serves for the tag now", http.StatusConflict
	}
	push.Digest = resolved
	if push.Digest == docker.ImageDigestOf(svc.Image) {
		return imageUpdateUpToDate, http.StatusOK
	}
	image := spelledRepo + ":" + push.Tag + "@" + push.Digest
	pending, err := rt.pendingApprovalFor(ctx, svc.Name, image)
	if err != nil {
		rt.logger.Warn("api: registry push: list pending approvals failed", slog.String("name", svc.Name), slog.String("error", err.Error()))
		return "push not deployed: could not check pending approvals", http.StatusServiceUnavailable
	}
	if pending {
		return "push skipped: an approval for " + shortDigest(push.Digest) + " is already waiting", http.StatusAccepted
	}
	if rt.environmentProtected(ctx, svc.EnvironmentID) {
		if _, err := rt.pushRotator(ctx, svc.Name); err != nil {
			return "push not deployed: rotate the registry webhook once so the approval can name who set it up", http.StatusConflict
		}
	}
	cw := &captureWriter{header: http.Header{}}
	rt.applyDeploy(cw, r, svc, deployTriggerRequest{Image: image, Confirm: true, Pull: true}, "")
	if cw.status >= 200 && cw.status < 300 {
		rt.supersedePushApprovals(r, svc.Name, image, *push)
	}
	return pushOutcome(cw, push.Digest), cw.status
}

func (rt *Router) resolvePushedDigest(ctx context.Context, svc store.DesiredService, spelledRepo, tag string) (string, error) {
	if rt.imageResolver == nil {
		return "", errors.New("no image resolver is configured on this control plane")
	}
	trigger := rt.imageTrigger(ctx, svc, true, "registry push")
	remote, err := rt.imageResolver.ResolveImage(ctx, spelledRepo+":"+tag, trigger.Auth, true)
	if err != nil {
		rt.logger.Warn("api: registry push: resolve failed", slog.String("name", svc.Name), slog.String("error", err.Error()))
		return "", errors.New("could not resolve the pushed tag from its registry")
	}
	if !pushDigestRE.MatchString(remote.Digest) {
		return "", errors.New("registry returned no digest for the pushed tag")
	}
	return remote.Digest, nil
}

func (rt *Router) pendingApprovalFor(ctx context.Context, name, image string) (bool, error) {
	if rt.deployApprovals == nil {
		return false, nil
	}
	pending, err := rt.deployApprovals.ListDeployApprovals(ctx, store.DeployApprovalStatusPending, name)
	if err != nil {
		return false, err
	}
	for _, a := range pending {
		if a.Image == image {
			return true, nil
		}
	}
	return false, nil
}

// promoteStagedSecret finishes a rotation whose promotion failed: a delivery signed with the staged secret proves the
// new owner has it. It runs under the rotation lock and only while the staged secret is still the one just verified.
func (rt *Router) promoteStagedSecret(ctx context.Context, name, secret string) {
	mu := rt.pushRotateLock(name)
	mu.Lock()
	defer mu.Unlock()
	staged, err := rt.gitSourceSecrets.Resolve(ctx, imageUpdateSecretNS+name, imageUpdateSecretPendingKey)
	if err != nil || staged != secret {
		return
	}
	rt.promotePushSecret(ctx, name, secret)
}

// promotePushSecret makes secret the live signing secret and empties the staged slot. The caller holds the rotation
// lock. Failure is logged and retried by the next signed delivery.
func (rt *Router) promotePushSecret(ctx context.Context, name, secret string) {
	if err := rt.gitSourceSecrets.SetValue(ctx, imageUpdateSecretNS+name, imageUpdateSecretKey, secret); err != nil {
		rt.logger.Warn("api: promote staged signing secret failed", slog.String("name", name), slog.String("error", err.Error()))
		return
	}
	if err := rt.gitSourceSecrets.SetValue(ctx, imageUpdateSecretNS+name, imageUpdateSecretPendingKey, pushPendingNone); err != nil {
		rt.logger.Warn("api: clear staged signing secret failed", slog.String("name", name), slog.String("error", err.Error()))
	}
}

// supersedePushApprovals rejects older pending approvals this hook filed for other digests, so approving a stale
// one cannot roll the app back past a newer push. Each one is audited with the id and the image it named.
func (rt *Router) supersedePushApprovals(r *http.Request, name, keepImage string, push registryPush) {
	if rt.deployApprovals == nil {
		return
	}
	// Detached from the run's deadline: a stale approval left pending could roll the app back past the newer push.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), rt.runtimeSettings.Duration(r.Context(), runtimesettings.KeyDeploysRegistryPushTimeout))
	defer cancel()
	pending, err := rt.deployApprovals.ListDeployApprovals(ctx, store.DeployApprovalStatusPending, name)
	if err != nil {
		rt.logger.Warn("api: supersede push approvals: list failed", slog.String("name", name), slog.String("error", err.Error()))
		return
	}
	for _, a := range pending {
		if a.RequestedByType != actorTypeRegistryPush || a.Image == keepImage {
			continue
		}
		if _, err := rt.rejectApproval(ctx, a.ID, actorTypeRegistryPush, name, push.Provider, "superseded by a newer push"); err != nil {
			rt.logger.Warn("api: supersede push approval failed", slog.String("name", name), slog.String("error", err.Error()))
			continue
		}
		rt.auditRegistryPushEvent(r, name, push, AuditActionRegistryPushSupersede, map[string]string{"approval_id": a.ID, "superseded_image": a.Image, "kept_image": keepImage}, http.StatusOK)
	}
}

// recordPushRotator remembers who last rotated the hook, so approvals the hook files can refuse that person as approver.
func (rt *Router) recordPushRotator(r *http.Request, name string) error {
	actorType, actorID, _, ok := rt.currentActor(r)
	if !ok {
		return errors.New("rotating the registry webhook needs an authenticated caller")
	}
	human, err := rt.approvalHuman(r.Context(), actorType, actorID)
	if err != nil {
		return err
	}
	if human == "" {
		human = pushRotatorNone
	}
	return rt.gitSourceSecrets.SetValue(r.Context(), imageUpdateSecretNS+name, imageUpdateRotatorKey, human)
}

// pushRotator is the person behind the hook's last rotation. Unknown means no approval may be filed for it.
func (rt *Router) pushRotator(ctx context.Context, name string) (string, error) {
	if rt.gitSourceSecrets == nil {
		return "", errApprovalOwnerUnknown
	}
	has, err := rt.gitSourceSecrets.Exists(ctx, imageUpdateSecretNS+name, imageUpdateRotatorKey)
	if err != nil || !has {
		return "", errApprovalOwnerUnknown
	}
	human, err := rt.gitSourceSecrets.Resolve(ctx, imageUpdateSecretNS+name, imageUpdateRotatorKey)
	if err != nil || human == "" || human == pushRotatorNone {
		return "", errApprovalOwnerUnknown
	}
	return human, nil
}

// pushOutcome turns what applyDeploy wrote into the one-line result the UI and CLI show.
func pushOutcome(cw *captureWriter, digest string) string {
	if cw.status >= 200 && cw.status < 300 {
		var res deployTriggerResult
		if json.Unmarshal(cw.body.Bytes(), &res) == nil && res.PendingApproval != nil {
			return "awaiting approval to deploy " + shortDigest(digest)
		}
		return "deployed pushed digest " + shortDigest(digest)
	}
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(cw.body.Bytes(), &e)
	if e.Error == "" {
		e.Error = http.StatusText(cw.status)
	}
	return fmt.Sprintf("push not deployed (%d): %s", cw.status, e.Error)
}

func (rt *Router) auditRegistryPush(r *http.Request, name string, push registryPush, outcome string, status int) {
	rt.auditRegistryPushEvent(r, name, push, AuditActionRegistryPushDeploy, map[string]string{
		"provider": push.Provider, "repository": push.Repo, "tag": push.Tag, "digest": push.Digest, "payload_digest": push.PayloadDigest, "outcome": outcome,
	}, status)
}

// auditRegistryPushEvent writes one audit row attributed to the delivery that caused it, never to the request that ran it.
func (rt *Router) auditRegistryPushEvent(r *http.Request, name string, push registryPush, action string, detail map[string]string, status int) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: registry push audit id failed", slog.String("error", err.Error()))
		return
	}
	raw, _ := json.Marshal(detail)
	entry := store.AuditEntry{
		ID: id, ActorType: actorTypeRegistryPush, ActorID: name, ActorName: push.Provider,
		Ability: AbilityDeploy, Method: http.MethodPost, Path: imageUpdateHookPrefix + name, StatusCode: status,
		RemoteAddr: push.RemoteAddr, CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(push.UserAgent),
		Action:     action, Detail: string(raw),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: registry push audit save failed", slog.String("error", err.Error()), slog.String("app", name))
		return
	}
	rt.emitAuditEvent(entry)
}
