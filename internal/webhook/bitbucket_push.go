package webhook

import (
	"encoding/json"
	"fmt"
)

// bitbucketEventKeyHeader is the header Bitbucket sets on every webhook
// delivery naming the event; "repo:push" is the one this package acts
// on, the same event GitHub's "push" and GitLab's "Push Hook"
// X-Gitlab-Event send, just under Bitbucket's own header/value scheme
// rather than a shared body field.
const bitbucketEventKeyHeader = "repo:push"

// bitbucketPushPayload is the subset of Bitbucket's repo:push webhook
// payload this package needs
// (https://support.atlassian.com/bitbucket-cloud/docs/event-payloads/#Push).
// Structurally unrelated to GitHub's and GitLab's shared ref/after
// fields (ParsePushEvent's own doc comment): Bitbucket nests one entry
// per updated ref under push.changes, each carrying a bare branch name
// (not a refs/heads/-prefixed ref) and the new head commit's hash, not
// "after". New is a pointer: a branch-delete change reports New as
// null, which this package must recognize rather than panic on.
type bitbucketPushPayload struct {
	Push struct {
		Changes []bitbucketPushChange `json:"changes"`
	} `json:"push"`
}

type bitbucketPushChange struct {
	New *bitbucketPushRef `json:"new"`
}

type bitbucketPushRef struct {
	Name   string              `json:"name"`
	Target bitbucketPushTarget `json:"target"`
}

type bitbucketPushTarget struct {
	Hash string `json:"hash"`
}

// ParseBitbucketPushEvent decodes body as a Bitbucket repo:push webhook
// payload, normalizing it into the same PushEvent shape ParsePushEvent
// produces for GitHub/GitLab: Ref gets Bitbucket's bare branch name
// prefixed with "refs/heads/" so Config.TargetRef's own comparison
// keeps working unchanged regardless of which provider sent the push.
// Picks the first change carrying a non-null New (the common case is
// exactly one), and fails with ErrPushEventFieldsMissing if every
// change in the payload is a branch delete (New null throughout): there
// is no commit to deploy in that case, the same "nothing meaningful to
// act on" reasoning that error already carries for a GitHub/GitLab
// payload missing ref or after.
func ParseBitbucketPushEvent(body []byte) (PushEvent, error) {
	var payload bitbucketPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return PushEvent{}, fmt.Errorf("webhook: malformed bitbucket payload: %w", err)
	}
	for _, change := range payload.Push.Changes {
		if change.New == nil || change.New.Name == "" || change.New.Target.Hash == "" {
			continue
		}
		return PushEvent{Ref: "refs/heads/" + change.New.Name, After: change.New.Target.Hash}, nil
	}
	return PushEvent{}, ErrPushEventFieldsMissing
}

// ParsePushEventForProvider dispatches to ParseBitbucketPushEvent when
// eventKeyHeader (the request's own X-Event-Key value) is Bitbucket's
// "repo:push", or ParsePushEvent otherwise: GitHub sends no such header
// (X-GitHub-Event instead, a different name this function deliberately
// never reads, so a GitHub delivery always falls through to the default
// case) and GitLab's own X-Gitlab-Event value ("Push Hook") never
// matches "repo:push" either. Centralizing the branch here, rather than
// in internal/api's own handler, keeps every provider's payload shape
// knowledge inside this package, the same "this package owns
// verified signature/payload logic" boundary handleGitPushWebhook's own
// doc comment already establishes.
func ParsePushEventForProvider(body []byte, eventKeyHeader string) (PushEvent, error) {
	if eventKeyHeader == bitbucketEventKeyHeader {
		return ParseBitbucketPushEvent(body)
	}
	return ParsePushEvent(body)
}
