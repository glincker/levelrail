package api

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// What attaching a repo to one service does to the other services of its app.
const (
	gitConnectMemberUnchanged = "unchanged"
	gitConnectMemberOwnSource = "keeps_own_source"
	gitConnectMemberBuiltWith = "built_with_this_source"
)

// gitSourcePlanMember is another service of the same app or compose stack.
type gitSourcePlanMember struct {
	Name          string `json:"name"`
	CurrentSource string `json:"current_source"`
	Effect        string `json:"effect"`
}

// planConnectionToken resolves a picked provider connection's token, only for
// a repo on that connection's own host so a pasted URL cannot carry it elsewhere.
func (rt *Router) planConnectionToken(ctx context.Context, repoURL, connectionID string) string {
	if rt.gitConns == nil {
		return ""
	}
	conn, err := rt.gitConns.GetGitConnection(ctx, connectionID)
	if err != nil {
		if !errors.Is(err, store.ErrGitConnectionNotFound) {
			rt.logger.Warn("api: git source plan: load connection failed", slog.String("connection_id", connectionID), slog.String("error", err.Error()))
		}
		return ""
	}
	if !repoOnConnectionInstance(repoURL, conn.InstanceURL) {
		return ""
	}
	_, token, err := rt.gitTokenForRepo(ctx, conn.Provider, repoURL, conn.ID)
	if err != nil {
		rt.logger.Info("api: git source plan: connection token unavailable", slog.String("connection_id", conn.ID), slog.String("error", err.Error()))
		return ""
	}
	return token
}

// repoOnConnectionInstance accepts only an https URL without userinfo whose
// exact host and port are the connection's instance, so a token never leaves it.
func repoOnConnectionInstance(repoURL, instanceURL string) bool {
	ru, err := url.Parse(repoURL)
	if err != nil || ru.Scheme != "https" || ru.User != nil || ru.Host == "" {
		return false
	}
	iu, err := url.Parse(instanceURL)
	if err != nil || iu.Scheme != "https" || iu.Host == "" {
		return false
	}
	return strings.EqualFold(ru.Host, iu.Host)
}

// planGroupMembers lists the app's sibling services the caller may read and
// what the switch does to each, so a multi-service app shows the blast radius.
func (rt *Router) planGroupMembers(ctx context.Context, svc store.DesiredService, canRead func(string) bool) []gitSourcePlanMember {
	out := []gitSourcePlanMember{}
	if svc.AppID == "" {
		return out
	}
	all, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		rt.logger.Warn("api: git source plan: list group failed", slog.String("name", svc.Name), slog.String("error", err.Error()))
		return out
	}
	var builtWith []string
	if gs, gerr := rt.gitSources.GetGitSource(ctx, svc.Name); gerr == nil {
		for n := range gs.AdditionalServices {
			builtWith = append(builtWith, n)
		}
	}
	for _, m := range all {
		if m.AppID != svc.AppID || m.Name == svc.Name || !canRead(m.Name) {
			continue
		}
		member := gitSourcePlanMember{Name: m.Name, CurrentSource: gitConnectSourceImage, Effect: gitConnectMemberUnchanged}
		if spec.IsPendingImage(m.Image) || m.Image == "" {
			member.CurrentSource = gitConnectSourceNone
		}
		if _, gerr := rt.gitSources.GetGitSource(ctx, m.Name); gerr == nil {
			member.CurrentSource, member.Effect = gitConnectSourceGit, gitConnectMemberOwnSource
		}
		if slices.Contains(builtWith, m.Name) {
			member.Effect = gitConnectMemberBuiltWith
		}
		out = append(out, member)
	}
	slices.SortFunc(out, func(a, b gitSourcePlanMember) int { return strings.Compare(a.Name, b.Name) })
	return out
}
