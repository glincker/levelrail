package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// MergeGroupEvent is the subset of GitHub's merge_group webhook payload a
// merge queue check needs.
type MergeGroupEvent struct {
	// Action is "checks_requested" when the queue wants checks to run, or
	// "destroyed" when the group is dissolved.
	Action  string
	HeadSHA string
	// HeadRef is the temporary queue branch, a full "refs/heads/..." ref.
	HeadRef string
	// BaseRef is the branch the group merges into, a full "refs/heads/..." ref.
	BaseRef string
	BaseSHA string
}

// BaseBranch is BaseRef without its "refs/heads/" prefix.
func (e MergeGroupEvent) BaseBranch() string { return strings.TrimPrefix(e.BaseRef, "refs/heads/") }

// ChecksRequested reports whether the queue is asking for checks.
func (e MergeGroupEvent) ChecksRequested() bool { return e.Action == "checks_requested" }

// IsMergeGroupEvent reports whether header names GitHub's merge_group event.
func IsMergeGroupEvent(header http.Header) bool {
	return header.Get("X-GitHub-Event") == "merge_group"
}

// ParseMergeGroupEvent decodes a GitHub merge_group payload.
func ParseMergeGroupEvent(body []byte) (MergeGroupEvent, error) {
	var p struct {
		Action     string `json:"action"`
		MergeGroup struct {
			HeadSHA string `json:"head_sha"`
			HeadRef string `json:"head_ref"`
			BaseRef string `json:"base_ref"`
			BaseSHA string `json:"base_sha"`
		} `json:"merge_group"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return MergeGroupEvent{}, fmt.Errorf("webhook: malformed merge_group payload: %w", err)
	}
	mg := p.MergeGroup
	if mg.HeadSHA == "" || mg.BaseRef == "" {
		return MergeGroupEvent{}, ErrPullRequestEventFieldsMissing
	}
	return MergeGroupEvent{Action: p.Action, HeadSHA: mg.HeadSHA, HeadRef: mg.HeadRef, BaseRef: mg.BaseRef, BaseSHA: mg.BaseSHA}, nil
}
