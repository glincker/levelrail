package gitprovider

import (
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

// AllowPrivateEnv lets git provider clients reach internal addresses, for
// example GitHub Enterprise or Bitbucket Server on a private network.
const AllowPrivateEnv = "APP_GIT_ALLOW_PRIVATE_NETWORKS"

// NewGuardedClient returns an SSRF-guarded HTTP client with the shared git provider timeout.
func NewGuardedClient() *http.Client {
	client := netguard.NewClientAllowing(AllowPrivateEnv)
	client.Timeout = 20 * time.Second
	return client
}
