package bitbucketapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

type prCommentResponse struct {
	ID      int64                  `json:"id"`
	Content createPRCommentContent `json:"content"`
}

type prCommentPage struct {
	Values []prCommentResponse `json:"values"`
	Next   string              `json:"next"`
}

func (c *Client) prCommentsURL(fullName string, prID int) string {
	return c.apiBaseURL() + repositoriesAPI + fullName + "/pullrequests/" + strconv.Itoa(prID) + "/comments"
}

// CreatePullRequestComment posts a new comment on pull request prID of
// fullName ("workspace/repo_slug") and returns its ID.
func (c *Client) CreatePullRequestComment(ctx context.Context, accessToken, fullName string, prID int, body string) (int64, error) {
	payload, err := json.Marshal(createPRCommentRequest{Content: createPRCommentContent{Raw: body}})
	if err != nil {
		return 0, fmt.Errorf("bitbucketapp: marshal pull request comment request: %w", err)
	}
	var out prCommentResponse
	if err := c.do(ctx, http.MethodPost, c.prCommentsURL(fullName, prID), bearerPrefix+accessToken, bytes.NewReader(payload), &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// ListPullRequestComments returns the comments of pull request prID,
// following Bitbucket's own next links up to gitprovider.MaxCommentPages pages.
func (c *Client) ListPullRequestComments(ctx context.Context, accessToken, fullName string, prID int) ([]gitprovider.Comment, error) {
	var all []gitprovider.Comment
	next := c.prCommentsURL(fullName, prID) + "?pagelen=" + strconv.Itoa(gitprovider.CommentPageSize)
	for page := 0; next != "" && page < gitprovider.MaxCommentPages; page++ {
		var batch prCommentPage
		if err := c.do(ctx, http.MethodGet, next, bearerPrefix+accessToken, nil, &batch); err != nil {
			return nil, err
		}
		for _, cm := range batch.Values {
			all = append(all, gitprovider.Comment{ID: cm.ID, Body: cm.Content.Raw})
		}
		next = batch.Next
	}
	return all, nil
}

// UpdatePullRequestComment replaces the body of an existing comment.
func (c *Client) UpdatePullRequestComment(ctx context.Context, accessToken, fullName string, prID int, commentID int64, body string) error {
	payload, err := json.Marshal(createPRCommentRequest{Content: createPRCommentContent{Raw: body}})
	if err != nil {
		return fmt.Errorf("bitbucketapp: marshal pull request comment request: %w", err)
	}
	u := c.prCommentsURL(fullName, prID) + "/" + strconv.FormatInt(commentID, 10)
	return c.do(ctx, http.MethodPut, u, bearerPrefix+accessToken, bytes.NewReader(payload), nil)
}
