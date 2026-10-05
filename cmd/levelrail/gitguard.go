package main

import (
	gitclient "github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/GLINCKER/levelrail/internal/netguard"
)

// installGitNetguard routes every go-git http(s) fetch through netguard, so a
// repo_url naming an internal host or redirecting to a metadata IP is refused.
func installGitNetguard() {
	c := githttp.NewClient(netguard.NewClient())
	gitclient.InstallProtocol("http", c)
	gitclient.InstallProtocol("https", c)
}
