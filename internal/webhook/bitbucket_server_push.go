package webhook

import (
	"encoding/json"
	"fmt"
	"strings"
)

// bitbucketServerRefsChanged is the X-Event-Key of a Bitbucket Server / Data Center push.
const bitbucketServerRefsChanged = "repo:refs_changed"

// bitbucketServerNullHash is the toHash Bitbucket Server reports for a deleted ref.
const bitbucketServerNullHash = "0000000000000000000000000000000000000000"

type bitbucketServerPushPayload struct {
	Repository struct {
		Slug    string `json:"slug"`
		Project struct {
			Key string `json:"key"`
		} `json:"project"`
		Links struct {
			Self []struct {
				Href string `json:"href"`
			} `json:"self"`
		} `json:"links"`
	} `json:"repository"`
	Changes []struct {
		Ref struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"ref"`
		FromHash string `json:"fromHash"`
		ToHash   string `json:"toHash"`
		Type     string `json:"type"`
	} `json:"changes"`
}

// ParseBitbucketServerPushEvent decodes a repo:refs_changed payload into a
// PushEvent. The first branch update or add wins; tag and delete changes carry
// nothing to deploy and yield ErrPushEventFieldsMissing when nothing else is left.
func ParseBitbucketServerPushEvent(body []byte) (PushEvent, error) {
	var p bitbucketServerPushPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return PushEvent{}, fmt.Errorf("webhook: malformed bitbucket server payload: %w", err)
	}
	for _, c := range p.Changes {
		if c.Type == "DELETE" || c.ToHash == "" || c.ToHash == bitbucketServerNullHash || !strings.HasPrefix(c.Ref.ID, "refs/heads/") {
			continue
		}
		ev := PushEvent{Ref: c.Ref.ID, After: c.ToHash}
		if c.FromHash != bitbucketServerNullHash {
			ev.Before = c.FromHash
		}
		if p.Repository.Project.Key != "" && p.Repository.Slug != "" {
			ev.RepoNames = []string{p.Repository.Project.Key + "/" + p.Repository.Slug}
		}
		if len(p.Repository.Links.Self) > 0 && p.Repository.Links.Self[0].Href != "" {
			ev.RepoURLs = []string{p.Repository.Links.Self[0].Href}
		}
		return ev, nil
	}
	return PushEvent{}, ErrPushEventFieldsMissing
}
