package api

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/store"
)

// previewCommenter resolves the comment surface of whichever provider hosts
// gs, or nil when comments are off or no connected provider matches.
func (rt *Router) previewCommenter(ctx context.Context, gs store.GitSource, p store.PreviewEnvironment) *prCommenter {
	n := p.PRNumber
	if inst, tok, owner, repo, ok := rt.previewGitHubTarget(ctx, p.AppName, gs); ok {
		c := rt.githubAppClient
		return &prCommenter{
			list: func(ctx context.Context) ([]gitprovider.Comment, error) {
				return c.ListIssueComments(ctx, inst, tok, owner, repo, n)
			},
			create: func(ctx context.Context, body string) (int64, error) {
				return c.CreateIssueComment(ctx, inst, tok, owner, repo, n, body)
			},
			update: func(ctx context.Context, id int64, body string) error {
				return c.UpdateIssueComment(ctx, inst, tok, owner, repo, id, body)
			},
		}
	}
	if inst, tok, project, ok := rt.previewGitLabTarget(ctx, p.AppName, gs); ok {
		c := rt.gitlabAppClient
		return &prCommenter{
			list: func(ctx context.Context) ([]gitprovider.Comment, error) {
				return c.ListMergeRequestNotes(ctx, inst, tok, project, n)
			},
			create: func(ctx context.Context, body string) (int64, error) {
				return c.CreateMergeRequestNote(ctx, inst, tok, project, n, body)
			},
			update: func(ctx context.Context, id int64, body string) error {
				return c.UpdateMergeRequestNote(ctx, inst, tok, project, n, id, body)
			},
		}
	}
	if tok, fullName, ok := rt.previewBitbucketTarget(ctx, p.AppName, gs); ok {
		c := rt.bitbucketAppClient
		return &prCommenter{
			list: func(ctx context.Context) ([]gitprovider.Comment, error) {
				return c.ListPullRequestComments(ctx, tok, fullName, n)
			},
			create: func(ctx context.Context, body string) (int64, error) {
				return c.CreatePullRequestComment(ctx, tok, fullName, n, body)
			},
			update: func(ctx context.Context, id int64, body string) error {
				return c.UpdatePullRequestComment(ctx, tok, fullName, n, id, body)
			},
		}
	}
	if inst, tok, fullName, ok := rt.previewGiteaTarget(ctx, p.AppName, gs); ok {
		c := rt.giteaAppClient
		return &prCommenter{
			list: func(ctx context.Context) ([]gitprovider.Comment, error) {
				return c.ListIssueComments(ctx, inst, tok, fullName, n)
			},
			create: func(ctx context.Context, body string) (int64, error) {
				return c.CreateIssueComment(ctx, inst, tok, fullName, n, body)
			},
			update: func(ctx context.Context, id int64, body string) error {
				return c.UpdateIssueComment(ctx, inst, tok, fullName, id, body)
			},
		}
	}
	return nil
}
