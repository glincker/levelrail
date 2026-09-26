package gitlabapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

type noteResponse struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

// CreateMergeRequestNote posts a new note on merge request mrIID of
// projectPath ("namespace/project") and returns its ID.
func (c *Client) CreateMergeRequestNote(ctx context.Context, instanceURL, accessToken, projectPath string, mrIID int, body string) (int64, error) {
	payload, err := json.Marshal(createNoteRequest{Body: body})
	if err != nil {
		return 0, fmt.Errorf("gitlabapp: marshal merge request note request: %w", err)
	}
	u := fmt.Sprintf("%s/projects/%s/merge_requests/%d/notes", apiBaseURL(instanceURL), encodedProjectID(projectPath), mrIID)
	var out noteResponse
	if err := c.do(ctx, http.MethodPost, u, "Bearer "+accessToken, bytes.NewReader(payload), &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// ListMergeRequestNotes returns the oldest notes of merge request mrIID, up
// to gitprovider.MaxCommentPages pages.
func (c *Client) ListMergeRequestNotes(ctx context.Context, instanceURL, accessToken, projectPath string, mrIID int) ([]gitprovider.Comment, error) {
	var all []gitprovider.Comment
	for page := 1; page <= gitprovider.MaxCommentPages; page++ {
		u := fmt.Sprintf("%s/projects/%s/merge_requests/%d/notes?order_by=created_at&sort=asc&per_page=%d&page=%d",
			apiBaseURL(instanceURL), encodedProjectID(projectPath), mrIID, gitprovider.CommentPageSize, page)
		var batch []noteResponse
		if err := c.do(ctx, http.MethodGet, u, "Bearer "+accessToken, nil, &batch); err != nil {
			return nil, err
		}
		for _, n := range batch {
			all = append(all, gitprovider.Comment{ID: n.ID, Body: n.Body})
		}
		if len(batch) < gitprovider.CommentPageSize {
			break
		}
	}
	return all, nil
}

// UpdateMergeRequestNote replaces the body of an existing note.
func (c *Client) UpdateMergeRequestNote(ctx context.Context, instanceURL, accessToken, projectPath string, mrIID int, noteID int64, body string) error {
	payload, err := json.Marshal(createNoteRequest{Body: body})
	if err != nil {
		return fmt.Errorf("gitlabapp: marshal merge request note request: %w", err)
	}
	u := fmt.Sprintf("%s/projects/%s/merge_requests/%d/notes/%d", apiBaseURL(instanceURL), encodedProjectID(projectPath), mrIID, noteID)
	return c.do(ctx, http.MethodPut, u, "Bearer "+accessToken, bytes.NewReader(payload), nil)
}
