package api

import (
	"context"
	"strings"

	"github.com/GLINCKER/levelrail/internal/webhook"
)

type commitMeta struct {
	Branch  string
	Message string
	Author  string
}

type commitMetaKey struct{}

// withPushCommitMeta carries a push's commit metadata to the deploy
// attempt row minted deeper in the call chain.
func withPushCommitMeta(ctx context.Context, ev webhook.PushEvent) context.Context {
	branch, _ := strings.CutPrefix(ev.Ref, "refs/heads/")
	if strings.HasPrefix(branch, "refs/") {
		branch = ""
	}
	return context.WithValue(ctx, commitMetaKey{}, commitMeta{
		Branch:  branch,
		Message: ev.HeadMessage,
		Author:  ev.HeadAuthor,
	})
}

func commitMetaFrom(ctx context.Context) commitMeta {
	m, _ := ctx.Value(commitMetaKey{}).(commitMeta)
	return m
}
