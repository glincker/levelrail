package giteaapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

type commentResponse struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

// CreateIssueComment posts a new comment on issue or pull request number of
// fullName ("owner/repo") and returns its ID.
func (c *Client) CreateIssueComment(ctx context.Context, instanceURL, accessToken, fullName string, number int, body string) (int64, error) {
	payload, err := json.Marshal(createIssueCommentRequest{Body: body})
	if err != nil {
		return 0, fmt.Errorf("giteaapp: marshal issue comment request: %w", err)
	}
	u := fmt.Sprintf("%s/repos/%s/issues/%d/comments", apiBaseURL(instanceURL), fullName, number)
	var out commentResponse
	if err := c.do(ctx, http.MethodPost, u, "Bearer "+accessToken, bytes.NewReader(payload), &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// ListIssueComments returns the comments of issue or pull request number,
// up to gitprovider.MaxCommentPages pages.
func (c *Client) ListIssueComments(ctx context.Context, instanceURL, accessToken, fullName string, number int) ([]gitprovider.Comment, error) {
	var all []gitprovider.Comment
	for page := 1; page <= gitprovider.MaxCommentPages; page++ {
		u := fmt.Sprintf("%s/repos/%s/issues/%d/comments?limit=%d&page=%d", apiBaseURL(instanceURL), fullName, number, gitprovider.CommentPageSize, page)
		var batch []commentResponse
		if err := c.do(ctx, http.MethodGet, u, "Bearer "+accessToken, nil, &batch); err != nil {
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
func (c *Client) UpdateIssueComment(ctx context.Context, instanceURL, accessToken, fullName string, commentID int64, body string) error {
	payload, err := json.Marshal(createIssueCommentRequest{Body: body})
	if err != nil {
		return fmt.Errorf("giteaapp: marshal issue comment request: %w", err)
	}
	u := fmt.Sprintf("%s/repos/%s/issues/comments/%d", apiBaseURL(instanceURL), fullName, commentID)
	return c.do(ctx, http.MethodPatch, u, "Bearer "+accessToken, bytes.NewReader(payload), nil)
}
