package webhook

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// IssueCommentEvent is the part of a GitHub issue_comment delivery that
// pull request comment commands need.
type IssueCommentEvent struct {
	Action        string
	CommentID     int64
	Body          string
	Commenter     string
	CommenterType string
	// PerformedViaApp is true when a GitHub App wrote the comment.
	PerformedViaApp bool
	SenderType      string
	Number          int
	// IsPullRequest is false for a comment on a plain issue.
	IsPullRequest bool
	RepoFullName  string
}

// ErrIssueCommentFieldsMissing is returned when the payload decodes but lacks
// a field the handler needs.
var ErrIssueCommentFieldsMissing = errors.New("webhook: issue_comment payload missing required fields")

// IsIssueCommentEvent reports whether header is a GitHub issue_comment
// delivery. Gitea sends GitHub-compatible headers too, so any other forge's
// event header disqualifies it.
func IsIssueCommentEvent(header http.Header) bool {
	if header.Get(HeaderGitHubEvent) != "issue_comment" {
		return false
	}
	return header.Get(HeaderGiteaEventType) == "" && header.Get(HeaderGitLabEvent) == "" && header.Get(HeaderBitbucketEventKey) == ""
}

type issueCommentPayload struct {
	Action string `json:"action"`
	Issue  struct {
		Number      int             `json:"number"`
		PullRequest json.RawMessage `json:"pull_request"`
	} `json:"issue"`
	Comment struct {
		ID                 int64           `json:"id"`
		Body               string          `json:"body"`
		PerformedViaGitHub json.RawMessage `json:"performed_via_github_app"`
		User               struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"user"`
	} `json:"comment"`
	Sender struct {
		Type string `json:"type"`
	} `json:"sender"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

// ParseIssueCommentEvent decodes a GitHub issue_comment payload.
func ParseIssueCommentEvent(body []byte) (IssueCommentEvent, error) {
	var p issueCommentPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return IssueCommentEvent{}, err
	}
	if p.Action == "" || p.Issue.Number <= 0 || p.Comment.ID <= 0 || p.Comment.User.Login == "" || p.Repository.FullName == "" {
		return IssueCommentEvent{}, ErrIssueCommentFieldsMissing
	}
	app := strings.TrimSpace(string(p.Comment.PerformedViaGitHub))
	return IssueCommentEvent{
		Action: p.Action, CommentID: p.Comment.ID, Body: p.Comment.Body,
		Commenter: p.Comment.User.Login, CommenterType: p.Comment.User.Type,
		PerformedViaApp: app != "" && app != "null",
		SenderType:      p.Sender.Type,
		Number:          p.Issue.Number,
		IsPullRequest:   len(p.Issue.PullRequest) > 0 && string(p.Issue.PullRequest) != "null",
		RepoFullName:    p.Repository.FullName,
	}, nil
}
