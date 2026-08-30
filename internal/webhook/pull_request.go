package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// PullRequestAction is a normalized pull-request lifecycle event,
// collapsing each provider's own action vocabulary (GitHub's
// opened/reopened/synchronize/closed, GitLab's open/reopen/update/
// close/merge, Bitbucket's created/updated/fulfilled/rejected event
// keys) down to the three this package's callers actually branch on.
type PullRequestAction string

const (
	// PullRequestOpened covers a pull request being opened or reopened:
	// a fresh preview deploy target.
	PullRequestOpened PullRequestAction = "opened"
	// PullRequestSynchronize covers new commits pushed to an
	// already-open pull request: redeploy its existing preview.
	PullRequestSynchronize PullRequestAction = "synchronize"
	// PullRequestClosed covers a pull request being closed, whether
	// merged or not: tear its preview down either way.
	PullRequestClosed PullRequestAction = "closed"
)

// PullRequestEvent is the subset of a provider's pull/merge request
// webhook payload this package needs to drive a preview environment.
type PullRequestEvent struct {
	Action PullRequestAction
	Number int
	// HeadRef is the pull request's source branch name, not a full
	// "refs/heads/"-prefixed ref: unlike PushEvent.Ref, no provider's
	// pull request payload carries one, and a preview deploy only ever
	// needs the branch name to name/label the preview, not to match it
	// against a target ref.
	HeadRef string
	HeadSHA string
	// BaseRef is the pull request's target branch name, compared against
	// a connected git source's own Branch the same way PushEvent.Ref is
	// compared against Config.TargetRef: a pull request against any
	// other branch is ignored.
	BaseRef string
}

// ErrPullRequestEventFieldsMissing is returned by a provider-specific
// parse function when the payload decodes as valid JSON but is missing
// a field this package needs, or names an action outside this package's
// normalized vocabulary (normalizePullRequestAction).
var ErrPullRequestEventFieldsMissing = errors.New("webhook: pull request payload missing required fields")

// normalizePullRequestAction maps a provider's own raw action string
// (or, for Bitbucket, its own event-key value) onto PullRequestAction.
// One shared table for all three providers: their vocabularies don't
// overlap in a way that would make a shared table ambiguous, so this
// avoids three near-identical switch statements silently drifting apart.
func normalizePullRequestAction(raw string) (PullRequestAction, bool) {
	switch raw {
	case "opened", "reopened", "open", "reopen", "pullrequest:created":
		return PullRequestOpened, true
	case "synchronize", "update", "pullrequest:updated":
		return PullRequestSynchronize, true
	case "closed", "close", "merge", "pullrequest:fulfilled", "pullrequest:rejected":
		return PullRequestClosed, true
	default:
		return "", false
	}
}

// IsPullRequestEvent reports whether header names a provider's own pull
// request / merge request event, so a caller can route to
// ParsePullRequestEventForProvider instead of ParsePushEventForProvider
// before looking at the body at all. GitHub and GitLab both send an
// unambiguous event-name header on every delivery (X-GitHub-Event,
// X-Gitlab-Event); Bitbucket reuses X-Event-Key, already read for its
// own push detection (ParsePushEventForProvider), just with a
// "pullrequest:"-prefixed value instead of "repo:push".
func IsPullRequestEvent(header http.Header) bool {
	if header.Get("X-GitHub-Event") == "pull_request" {
		return true
	}
	if header.Get("X-Gitlab-Event") == "Merge Request Hook" {
		return true
	}
	return strings.HasPrefix(header.Get("X-Event-Key"), "pullrequest:")
}

// ParsePullRequestEventForProvider dispatches to the right provider's
// own payload shape based on header, the same header-driven dispatch
// IsPullRequestEvent already uses to decide this function should even
// be called.
func ParsePullRequestEventForProvider(body []byte, header http.Header) (PullRequestEvent, error) {
	if header.Get("X-Gitlab-Event") == "Merge Request Hook" {
		return parseGitLabPullRequestEvent(body)
	}
	if eventKey := header.Get("X-Event-Key"); strings.HasPrefix(eventKey, "pullrequest:") {
		return parseBitbucketPullRequestEvent(body, eventKey)
	}
	return parseGitHubPullRequestEvent(body)
}

// githubPullRequestPayload is the subset of GitHub's pull_request event
// payload this package needs.
// https://docs.github.com/en/webhooks/webhook-events-and-payloads#pull_request
type githubPullRequestPayload struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		Head struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	} `json:"pull_request"`
}

func parseGitHubPullRequestEvent(body []byte) (PullRequestEvent, error) {
	var p githubPullRequestPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return PullRequestEvent{}, fmt.Errorf("webhook: malformed github pull_request payload: %w", err)
	}
	action, ok := normalizePullRequestAction(p.Action)
	if !ok || p.Number == 0 || p.PullRequest.Head.SHA == "" {
		return PullRequestEvent{}, ErrPullRequestEventFieldsMissing
	}
	return PullRequestEvent{
		Action: action, Number: p.Number,
		HeadRef: p.PullRequest.Head.Ref, HeadSHA: p.PullRequest.Head.SHA,
		BaseRef: p.PullRequest.Base.Ref,
	}, nil
}

// gitlabPullRequestPayload is the subset of GitLab's Merge Request Hook
// payload this package needs.
// https://docs.gitlab.com/user/project/integrations/webhook_events/#merge-request-events
type gitlabPullRequestPayload struct {
	ObjectAttributes struct {
		IID          int    `json:"iid"`
		Action       string `json:"action"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
		LastCommit   struct {
			ID string `json:"id"`
		} `json:"last_commit"`
	} `json:"object_attributes"`
}

func parseGitLabPullRequestEvent(body []byte) (PullRequestEvent, error) {
	var p gitlabPullRequestPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return PullRequestEvent{}, fmt.Errorf("webhook: malformed gitlab merge request payload: %w", err)
	}
	action, ok := normalizePullRequestAction(p.ObjectAttributes.Action)
	if !ok || p.ObjectAttributes.IID == 0 || p.ObjectAttributes.LastCommit.ID == "" {
		return PullRequestEvent{}, ErrPullRequestEventFieldsMissing
	}
	return PullRequestEvent{
		Action: action, Number: p.ObjectAttributes.IID,
		HeadRef: p.ObjectAttributes.SourceBranch, HeadSHA: p.ObjectAttributes.LastCommit.ID,
		BaseRef: p.ObjectAttributes.TargetBranch,
	}, nil
}

// bitbucketPullRequestPayload is the subset of Bitbucket's pullrequest:*
// event payloads this package needs.
// https://support.atlassian.com/bitbucket-cloud/docs/event-payloads/#Pull-request-created
type bitbucketPullRequestPayload struct {
	PullRequest struct {
		ID     int `json:"id"`
		Source struct {
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
			Commit struct {
				Hash string `json:"hash"`
			} `json:"commit"`
		} `json:"source"`
		Destination struct {
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
		} `json:"destination"`
	} `json:"pullrequest"`
}

func parseBitbucketPullRequestEvent(body []byte, eventKey string) (PullRequestEvent, error) {
	var p bitbucketPullRequestPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return PullRequestEvent{}, fmt.Errorf("webhook: malformed bitbucket pullrequest payload: %w", err)
	}
	action, ok := normalizePullRequestAction(eventKey)
	if !ok || p.PullRequest.ID == 0 || p.PullRequest.Source.Commit.Hash == "" {
		return PullRequestEvent{}, ErrPullRequestEventFieldsMissing
	}
	return PullRequestEvent{
		Action: action, Number: p.PullRequest.ID,
		HeadRef: p.PullRequest.Source.Branch.Name, HeadSHA: p.PullRequest.Source.Commit.Hash,
		BaseRef: p.PullRequest.Destination.Branch.Name,
	}, nil
}
