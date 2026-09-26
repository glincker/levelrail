package githubapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

type commentResponse struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

// CreateIssueComment posts a new comment on issue or pull request number
// (GitHub serves both from the issues endpoint) and returns its ID.
func (c *Client) CreateIssueComment(ctx context.Context, instanceURL, token, owner, repo string, number int, body string) (int64, error) {
	payload, err := json.Marshal(createIssueCommentRequest{Body: body})
	if err != nil {
		return 0, fmt.Errorf("githubapp: marshal issue comment request: %w", err)
	}
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", url.PathEscape(owner), url.PathEscape(repo), number)
	var out commentResponse
	if err := c.doJSON(ctx, c.APIBaseURL(instanceURL), http.MethodPost, path, bearerPrefix+token, payload, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// ListIssueComments returns the oldest comments of issue or pull request
// number, up to gitprovider.MaxCommentPages pages.
func (c *Client) ListIssueComments(ctx context.Context, instanceURL, token, owner, repo string, number int) ([]gitprovider.Comment, error) {
	var all []gitprovider.Comment
	for page := 1; page <= gitprovider.MaxCommentPages; page++ {
		path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments?per_page=%d&page=%d", url.PathEscape(owner), url.PathEscape(repo), number, gitprovider.CommentPageSize, page)
		var batch []commentResponse
		if err := c.doJSON(ctx, c.APIBaseURL(instanceURL), http.MethodGet, path, bearerPrefix+token, nil, &batch); err != nil {
			return nil, err
		}
		for _, cm := range batch {
			all = append(all, gitprovider.Comment{ID: cm.ID, Body: cm.Body})
		}
		if len(batch) < gitprovider.CommentPageSize {
			break
		}
	}
	return all, nil
}

// UpdateIssueComment replaces the body of an existing comment.
func (c *Client) UpdateIssueComment(ctx context.Context, instanceURL, token, owner, repo string, commentID int64, body string) error {
	payload, err := json.Marshal(createIssueCommentRequest{Body: body})
	if err != nil {
		return fmt.Errorf("githubapp: marshal issue comment request: %w", err)
	}
	path := fmt.Sprintf("/repos/%s/%s/issues/comments/%d", url.PathEscape(owner), url.PathEscape(repo), commentID)
	return c.doJSON(ctx, c.APIBaseURL(instanceURL), http.MethodPatch, path, bearerPrefix+token, payload, nil)
}
