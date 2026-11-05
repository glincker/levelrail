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
	pushRotatorNone    = "none"
	headerGitHubEvent  = "X-GitHub-Event"
	headerGitHubSig256 = "X-Hub-Signature-256"
	// ActorTypeRegistryPush labels deploys a registry push started, in audit rows and approvals.
	actorTypeRegistryPush = "registry_push"
	// AuditActionRegistryPushDeploy marks the outcome of one registry push delivery that passed authentication.
	AuditActionRegistryPushDeploy = "registry_push.deploy"
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
func (rt *Router) handleRegistryPushDelivery(w http.ResponseWriter, r *http.Request, name string) (handled bool) {
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
	rt.startRegistryPush(w, r, name, push)
	return true
}

// verifyPushSignature enforces the optional GitHub HMAC. With no secret set the URL token is the only gate.
// A secret staged by a rotation that has not finished is accepted too.
func (rt *Router) verifyPushSignature(w http.ResponseWriter, r *http.Request, name string, body []byte) (ok, signed bool) {
	if rt.gitSourceSecrets == nil {
		return true, false
	}
	var secrets []string
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
		secrets = append(secrets, secret)
	}
	if len(secrets) == 0 {
		return true, false
	}
	for _, secret := range secrets {
		if verifyGitHubSignature(secret, r.Header.Get(headerGitHubSig256), body) {
			return true, true
		}
	}
	rt.logger.Warn("api: image update webhook: bad signature", slog.String("name", name), slog.String("remote_addr", clientIP(r)))
	writeError(w, http.StatusNotFound, "not found")
	return false, false
}

// pushSlot serializes push deploys per app. A delivery that arrives while one runs is kept as next and runs after it,
// so the latest push is never lost; each run resolves the registry afresh, so a coalesced run is idempotent.
type pushSlot struct {
	mu      sync.Mutex
	running bool
	next    *registryPush
}

func (rt *Router) registryPushSlot(name string) *pushSlot {
	slot, _ := rt.registryPushLocks.LoadOrStore(name, &pushSlot{})
	return slot.(*pushSlot)
}

// startRegistryPush checks the push belongs to this app, then deploys its digest in the background.
func (rt *Router) startRegistryPush(w http.ResponseWriter, r *http.Request, name string, push registryPush) {
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
		u, uerr := rt.imageAutoUpdates.GetImageAutoUpdate(r.Context(), name)
		if uerr == nil {
			reason = rt.imagePushHoldReason(r.Context(), *svc, u, push)
		}
	}
	if reason != "" {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "ignored: " + reason})
		return
	}
	slot := rt.registryPushSlot(name)
	slot.mu.Lock()
	if slot.running {
		slot.next = &push
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

// imagePushHoldReason applies the auto-update apply window and the debounce. A protected environment skips the
// window because a person approves anyway. A delivery that costs a registry lookup is debounced; a signed one
// only when it repeats the digest of the last result, so real back-to-back builds still go through.
func (rt *Router) imagePushHoldReason(ctx context.Context, svc store.DesiredService, u store.ImageAutoUpdate, push registryPush) string {
	if !rt.environmentProtected(ctx, svc.EnvironmentID) && !inUpdateWindow(u.WindowCron, u.WindowMinutes, time.Now()) {
		if recErr := rt.imageAutoUpdates.RecordImageAutoUpdateCheck(ctx, svc.Name, imageUpdateWaiting, time.Now()); recErr != nil {
			rt.logger.Warn("api: record registry push wait failed", slog.String("name", svc.Name), slog.String("error", recErr.Error()))
		}
		return "outside the update window"
	}
	if u.LastCheckedAt == nil || time.Since(*u.LastCheckedAt) >= rt.runtimeSettings.Duration(ctx, runtimesettings.KeyDeploysRegistryPushDebounce) {
		return ""
	}
	if !push.signed || (push.Digest != "" && strings.Contains(u.LastResult, shortDigest(push.Digest))) {
		return "checked recently"
	}
	return ""
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
	spelled, normalized, _, err := splitImageRef(svc.Image)
	if err != nil || normalized != push.Repo {
		return "push not deployed: the app no longer uses this repository", http.StatusConflict
	}
	return rt.deployPushedDigest(r, *svc, spelled, push)
}

func (rt *Router) deployPushedDigest(r *http.Request, svc store.DesiredService, spelledRepo string, push *registryPush) (string, int) {
	ctx := r.Context()
	// The registry is the truth: a payload digest is only a claim, and a replayed or forged one must not pick the image.
	resolved, err := rt.resolvePushedDigest(ctx, svc, spelledRepo, push.Tag)
	if err != nil {
		rt.logger.Warn("api: registry push: resolve digest failed", slog.String("name", svc.Name), slog.String("error", err.Error()))
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
		rt.supersedePushApprovals(ctx, svc.Name, image, push.Provider)
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

// supersedePushApprovals rejects older pending approvals this hook filed for other digests, so approving a stale
// one cannot roll the app back past a newer push.
func (rt *Router) supersedePushApprovals(ctx context.Context, name, keepImage, provider string) {
	if rt.deployApprovals == nil {
		return
	}
	pending, err := rt.deployApprovals.ListDeployApprovals(ctx, store.DeployApprovalStatusPending, name)
	if err != nil {
		return
	}
	now := store.FormatAuditTime(time.Now())
	for _, a := range pending {
		if a.RequestedByType != actorTypeRegistryPush || a.Image == keepImage {
			continue
		}
		if _, err := rt.deployApprovals.DecideDeployApproval(ctx, a.ID, store.DeployApprovalStatusRejected, actorTypeRegistryPush, name, provider, "superseded by a newer push", now); err != nil {
			rt.logger.Warn("api: supersede push approval failed", slog.String("name", name), slog.String("error", err.Error()))
		}
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
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: registry push audit id failed", slog.String("error", err.Error()))
		return
	}
	detail, _ := json.Marshal(map[string]string{
		"provider": push.Provider, "repository": push.Repo, "tag": push.Tag, "digest": push.Digest, "payload_digest": push.PayloadDigest, "outcome": outcome,
	})
	entry := store.AuditEntry{
		ID: id, ActorType: actorTypeRegistryPush, ActorID: name, ActorName: push.Provider,
		Ability: AbilityDeploy, Method: http.MethodPost, Path: imageUpdateHookPrefix + name, StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     AuditActionRegistryPushDeploy, Detail: string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: registry push audit save failed", slog.String("error", err.Error()), slog.String("app", name))
		return
	}
	rt.emitAuditEvent(entry)
}
