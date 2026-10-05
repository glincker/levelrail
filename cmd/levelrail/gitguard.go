package main

import (
	gitclient "github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/GLINCKER/levelrail/internal/netguard"
)

// gitAllowPrivateEnv lets git fetches reach private addresses (a LAN Gitea)
// without also relaxing the notification guard.
const gitAllowPrivateEnv = "APP_GIT_ALLOW_PRIVATE_NETWORKS"

// installGitNetguard routes every go-git http(s) fetch through netguard, so a
// repo_url naming an internal host or redirecting to a metadata IP is refused.
// The shared notification override keeps working for existing installs.
func installGitNetguard() {
	c := githttp.NewClient(netguard.NewClientAllowing(gitAllowPrivateEnv, netguard.AllowPrivateEnv))
	gitclient.InstallProtocol("http", c)
	gitclient.InstallProtocol("https", c)
}
