package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

const (
	webhookNoteDuplicate    = "duplicate delivery"
	webhookNoteDeployed     = "deployed"
	webhookNoteReplayedBody = "replayed body"
	webhookSHALen           = 12
)

// webhookOutcome lets the push path report the attempt it queued to the
// handler that persists the delivery row.
type webhookOutcome struct {
	AttemptID string
}

type (
	webhookOutcomeKey struct{}
	webhookReplayKey  struct{}
)

func withWebhookOutcome(ctx context.Context) (context.Context, *webhookOutcome) {
	o := &webhookOutcome{}
	return context.WithValue(ctx, webhookOutcomeKey{}, o), o
}

func webhookOutcomeFrom(ctx context.Context) *webhookOutcome {
	o, _ := ctx.Value(webhookOutcomeKey{}).(*webhookOutcome)
	return o
}

// withWebhookReplay marks an operator-initiated replay, which skips the
// "already deployed" suppression because re-running is the point.
func withWebhookReplay(ctx context.Context) context.Context {
	return context.WithValue(ctx, webhookReplayKey{}, true)
}

func isWebhookReplay(ctx context.Context) bool {
	v, _ := ctx.Value(webhookReplayKey{}).(bool)
	return v
}

// webhookDeliveryID returns the provider's own delivery identifier, empty
// when the request carries none.
func webhookDeliveryID(h http.Header) string {
	for _, k := range []string{webhook.HeaderGitHubDelivery, webhook.HeaderGitLabEventUUID, webhook.HeaderGiteaDelivery, webhook.HeaderBitbucketRequest} {
		if v := strings.TrimSpace(h.Get(k)); v != "" {
			if len(v) > 128 {
				v = v[:128]
			}
			return v
		}
	}
	return ""
}

// repoIdentity reduces a repository URL or "owner/name" to a lowercase
// path without scheme, host, credentials, ".git" suffix or trailing slash,
// so https, ssh and scp-style forms and nested GitLab groups compare equal.
func repoIdentity(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if i := strings.Index(s, "://"); i >= 0 {
		if u, err := url.Parse(s); err == nil {
			s = u.Path
		} else {
			s = s[i+3:]
			if j := strings.Index(s, "/"); j >= 0 {
				s = s[j:]
			}
		}
	} else if at := strings.Index(s, "@"); at >= 0 {
		s = s[at+1:]
		if c := strings.Index(s, ":"); c >= 0 {
			s = s[c+1:]
		}
	}
	s = strings.Trim(s, "/")
	s = strings.TrimSuffix(s, ".git")
	return strings.ToLower(strings.Trim(s, "/"))
}

// pushRepoMatches reports whether the push is for the repository tracked.
// A payload that names no repository is treated as a match, since there
// is nothing to compare. pushed is the identity shown in the ignore note.
func pushRepoMatches(ev webhook.PushEvent, trackedURL string) (ok bool, pushed string) {
	want := repoIdentity(trackedURL)
	var ids []string
	for _, n := range ev.RepoNames {
		ids = append(ids, repoIdentity(n))
	}
	for _, u := range ev.RepoURLs {
		ids = append(ids, repoIdentity(u))
	}
	if len(ids) == 0 || want == "" {
		return true, ""
	}
	for _, id := range ids {
		if id != "" && id == want {
			return true, id
		}
	}
	if len(ev.RepoNames) > 0 {
		return false, repoIdentity(ev.RepoNames[0])
	}
	return false, ids[0]
}

func webhookShortSHA(sha string) string {
	if len(sha) > webhookSHALen {
		return sha[:webhookSHALen]
	}
	return sha
}

// commitDeployState returns "deployed" or "in flight" when the newest
// succeeded, queued or running attempt of name is for sha, else "". Only
// the newest counts so re-pushing a commit after a rollback still deploys.
func (rt *Router) commitDeployState(ctx context.Context, name, sha string) string {
	if sha == "" || rt.deployAttempts == nil {
		return ""
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
	if err != nil {
		rt.logger.Warn("api: git push webhook: list attempts for dedupe failed", slog.String("error", err.Error()), slog.String("name", name))
		return ""
	}
	sort.SliceStable(attempts, func(i, j int) bool { return attempts[i].StartedAt.After(attempts[j].StartedAt) })
	for _, a := range attempts {
		switch a.Status {
		case store.DeployAttemptStatusSucceeded:
			if a.CommitSHA == sha {
				return "deployed"
			}
			return ""
		case store.DeployAttemptStatusQueued, store.DeployAttemptStatusRunning:
			if a.CommitSHA == sha {
				return "in flight"
			}
			return ""
		}
	}
	return ""
}

// enqueueGitPushDeploy records the push as a queued attempt and returns
// at once; DrainDeployQueue runs the clone and build on a detached
// context with the queue's per-app serialization and concurrency cap.
func (rt *Router) enqueueGitPushDeploy(ctx context.Context, name string, hr deploy.HeldRequest) (int, string, bool) {
	existing, err := rt.apps.GetDesiredService(ctx, name)
	if errors.Is(err, store.ErrServiceNotFound) {
		return http.StatusNotFound, "app not found", true
	}
	if err != nil {
		rt.logger.Error("api: git push webhook: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		return http.StatusInternalServerError, "internal error", true
	}
	rt.buildStartMu.Lock()
	id, err := rt.enqueueDeploy(ctx, *existing, store.DeployAttemptSourceWebhook, name+":"+hr.CommitSHA, hr)
	rt.buildStartMu.Unlock()
	if err != nil {
		rt.logger.Error("api: git push webhook: enqueue failed", slog.String("error", err.Error()), slog.String("name", name))
		return 0, "", false
	}
	if o := webhookOutcomeFrom(ctx); o != nil {
		o.AttemptID = id
	} else {
		rt.kickDeployQueue(ctx)
	}
	return http.StatusAccepted, fmt.Sprintf("deploy queued: %s\n", id), true
}

// kickDeployQueue starts any queued deploy that is no longer blocked.
func (rt *Router) kickDeployQueue(ctx context.Context) {
	rt.DrainDeployQueue(context.WithoutCancel(ctx))
}

type webhookOutcomeUpdater interface {
	UpdateWebhookDeliveryOutcomeByAttempt(ctx context.Context, attemptID, note, errMsg string) error
}

type webhookDeliveryFinder interface {
	FindWebhookDeliveryByDeliveryID(ctx context.Context, serviceName, deliveryID string) (*store.WebhookDelivery, error)
}

// recordWebhookDeliveryOutcome updates the delivery that queued attemptID
// once the background deploy ends.
func (rt *Router) recordWebhookDeliveryOutcome(ctx context.Context, attemptID, status, msg string) {
	u, ok := rt.webhookDeliveries.(webhookOutcomeUpdater)
	if !ok || attemptID == "" {
		return
	}
	note, errMsg := webhookNoteDeployed, ""
	if status != store.DeployAttemptStatusSucceeded {
		errMsg = webhookDeliveryNote("deploy failed: " + msg)
		note = ""
	}
	if err := u.UpdateWebhookDeliveryOutcomeByAttempt(ctx, attemptID, note, errMsg); err != nil {
		rt.logger.Warn("api: update webhook delivery outcome failed", slog.String("attempt_id", attemptID), slog.String("error", err.Error()))
	}
}

// duplicateWebhookDelivery returns the earlier verified delivery with the
// same provider id, unless it failed (a provider "redeliver" of a failure
// reuses the id and must be processed).
func (rt *Router) duplicateWebhookDelivery(ctx context.Context, name, deliveryID string) *store.WebhookDelivery {
	f, ok := rt.webhookDeliveries.(webhookDeliveryFinder)
	if !ok || deliveryID == "" {
		return nil
	}
	d, err := f.FindWebhookDeliveryByDeliveryID(ctx, name, deliveryID)
	if err != nil || d == nil || d.StatusCode >= http.StatusBadRequest || d.Error != "" {
		return nil
	}
	return d
}
