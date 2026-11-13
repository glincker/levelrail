package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/gitprovider/reporef"
)

// envGitSourceHint set to off, false, 0 or no hides the "no git source" line after a deploy.
const envGitSourceHint = "APP_GIT_SOURCE_HINT"

const gitSourceHintDefaultBranch = "main"

type gitSourceGetter interface {
	GetGitSource(ctx context.Context, name string) (apiclient.GitSourceResource, error)
}

func gitSourceHintEnabled(lookupEnv func(string) (string, bool)) bool {
	v, ok := lookupEnv(envGitSourceHint)
	if !ok {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "off", "false", "0", "no":
		return false
	}
	return true
}

// gitSourceHintLine is the suggestion for a checkout whose remote the parser can
// resolve; the repo is printed as its credential-free clone URL.
func gitSourceHintLine(prog, app string, git detectedGit) string {
	r, err := reporef.Parse(git.RepoURL)
	if err != nil {
		return ""
	}
	branch := git.Ref
	if branch == "" {
		branch = gitSourceHintDefaultBranch
	}
	return fmt.Sprintf("This app has no git source, so pushes will not deploy. Connect it: %s apps git-source set %s --repo %s --branch %s", prog, app, r.CloneURL(), branch)
}

// printGitSourceHint prints the hint only when the server confirms the app has
// no git source; any other answer is not proof, so it stays quiet.
func printGitSourceHint(ctx context.Context, client gitSourceGetter, prog, app string, git detectedGit, lookupEnv func(string) (string, bool), w io.Writer) {
	if !gitSourceHintEnabled(lookupEnv) {
		return
	}
	line := gitSourceHintLine(prog, app, git)
	if line == "" {
		return
	}
	_, err := client.GetGitSource(ctx, app)
	var apiErr *apiclient.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		return
	}
	_, _ = fmt.Fprintln(w, line)
}
