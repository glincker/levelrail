package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

const gitConnectionsDocsPath = "/git-integrations#reconnecting-a-provider"

// doctorCheckGitConnections warns when a connected git provider needs to be
// reconnected, so a revoked grant shows up before the next failed deploy.
func (rt *Router) doctorCheckGitConnections(ctx context.Context) doctorCheckResource {
	const code, name = "git_connections", "Git provider connections"
	res := doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, DocsPath: gitConnectionsDocsPath}
	statuses := []gitProviderResource{rt.githubProviderStatus(ctx)}
	for _, p := range connectionGitProviders {
		statuses = append(statuses, rt.gitProviderStatusFor(ctx, p))
	}
	var connected, problems []string
	for _, p := range statuses {
		if !p.Connected {
			continue
		}
		connected = append(connected, p.Provider)
		h := rt.gitProviderHealth(ctx, p.Provider)
		if h.Status == store.GitConnOK {
			continue
		}
		problems = append(problems, fmt.Sprintf("%s (%s)", p.Provider, h.Status))
	}
	switch {
	case len(connected) == 0:
		res.Message = "no git provider is connected"
	case len(problems) > 0:
		res.Status = doctorStatusWarn
		res.Message = strings.Join(problems, ", ") + ": press Reconnect on the provider's settings page"
		res.Evidence = strings.Join(problems, ", ")
	default:
		res.Message = strings.Join(connected, ", ") + " connected and accepted by the provider"
	}
	return res
}
