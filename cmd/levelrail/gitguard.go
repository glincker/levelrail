package main

import (
	"context"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	gitclient "github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/GLINCKER/levelrail/internal/gitsshkey"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// gitAllowPrivateEnv lets git fetches reach private addresses (a LAN Gitea)
// without also relaxing the notification guard.
const gitAllowPrivateEnv = "APP_GIT_ALLOW_PRIVATE_NETWORKS"

// gitSSHResolveTimeoutEnv bounds the host lookup before an ssh fetch; the
// host key scan reads the same variable.
const gitSSHResolveTimeoutEnv = "APP_GIT_SSH_SCAN_TIMEOUT"

// gitGuardSettings is read on every ssh session. Boot stores a fallback, then
// the API's shared service, whose cache its save handlers clear.
var gitGuardSettings atomic.Pointer[runtimesettings.Service]

// installGitNetguard routes every go-git http(s) fetch through netguard and
// every ssh fetch through the deploy key and internal-address guard. The
// ssh allowlist is the live git.ssh_scan_allow_internal setting.
func installGitNetguard(fallback *runtimesettings.Service) {
	gitGuardSettings.Store(fallback)
	c := githttp.NewClient(netguard.NewClientAllowing(gitAllowPrivateEnv, netguard.AllowPrivateEnv))
	gitclient.InstallProtocol("http", c)
	gitclient.InstallProtocol("https", c)
	gitsshkey.InstallGuard(gitsshkey.GuardOptions{
		Allow:          gitSSHAllowList,
		AllowPrivate:   func() bool { return envTrue(gitAllowPrivateEnv) || netguard.AllowPrivate() },
		ResolveTimeout: envDuration(gitSSHResolveTimeoutEnv, 10*time.Second),
	})
}

// useSharedGitSettings points the fetch guard at the API's settings service,
// so a saved allowlist applies without a restart.
func useSharedGitSettings(shared *runtimesettings.Service) {
	if shared != nil {
		gitGuardSettings.Store(shared)
	}
}

func gitSSHAllowList() []string {
	settings := gitGuardSettings.Load()
	if settings == nil {
		return nil
	}
	return settings.List(context.Background(), runtimesettings.KeyGitSSHScanAllowInternal)
}

func envTrue(name string) bool {
	v, err := strconv.ParseBool(os.Getenv(name))
	return err == nil && v
}
