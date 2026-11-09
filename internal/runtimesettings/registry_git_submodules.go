package runtimesettings

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Keys bounding submodule and LFS fetches of git source builds.
const (
	KeyGitSubmodulesMaxDepth     = "git.submodules_max_depth"
	KeyGitSubmodulesMaxCount     = "git.submodules_max_count"
	KeyGitSubmodulesMaxMB        = "git.submodules_max_mb"
	KeyGitSubmodulesTimeout      = "git.submodules_timeout"
	KeyGitSubmodulesAuthHosts    = "git.submodules_credential_hosts"
	KeyGitLFSHosts               = "git.lfs_hosts"
	KeyGitSubmodulesAllowedCIDRs = "git.submodules_allowed_cidrs"

	gitSubmodulesTimeoutFloor = 10 * time.Second
	gitSubmodulesTimeoutCeil  = time.Hour
	gitSubmodulesTimeoutDef   = 10 * time.Minute
)

// hostnameCheck accepts a bare host name or host:port, never a URL.
func hostnameCheck(item string) error {
	item = strings.TrimSpace(item)
	if item == "" || strings.ContainsAny(item, "/@ \t") {
		return fmt.Errorf("%q must be a host name such as git.example.com", item)
	}
	return nil
}

func init() {
	registerLimit(Definition{
		Key: KeyGitSubmodulesMaxDepth, Category: CategoryDeploys, Default: "3", Env: "APP_GIT_SUBMODULES_MAX_DEPTH", Apply: ApplyLive,
		Title:       "Submodule nesting depth",
		Description: "How deep submodules of a git source build may nest, from 1 to 10. A deeper tree fails the build with a clear error.",
	}, 1, 10)
	registerLimit(Definition{
		Key: KeyGitSubmodulesMaxCount, Category: CategoryDeploys, Default: "32", Env: "APP_GIT_SUBMODULES_MAX_COUNT", Apply: ApplyLive,
		Title:       "Submodules per build",
		Description: "Most submodules, at every depth together, one git source build fetches, from 1 to 256.",
	}, 1, 256)
	registerLimit(Definition{
		Key: KeyGitSubmodulesMaxMB, Category: CategoryDeploys, Default: "4096", Env: "APP_GIT_SUBMODULES_MAX_MB", Apply: ApplyLive,
		Title:       "Checkout size with submodules and LFS",
		Description: "Largest checkout in MB, repository plus submodules plus LFS objects, from 16 to 65536. A bigger checkout fails the build.",
	}, 16, 65536)
	Register(Definition{
		Key: KeyGitSubmodulesTimeout, Category: CategoryDeploys, Kind: KindDuration, Default: "10m", Env: "APP_GIT_SUBMODULES_TIMEOUT", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(gitSubmodulesTimeoutFloor, gitSubmodulesTimeoutCeil),
		Title:       "Submodule and LFS fetch timeout",
		Description: "How long fetching all submodules and LFS objects of one build may take, from 10s to 1h.",
	})
	Register(Definition{
		Key: KeyGitSubmodulesAuthHosts, Category: CategorySecurity, Kind: KindStringList, Default: "",
		Env: "APP_GIT_SUBMODULES_CREDENTIAL_HOSTS", Apply: ApplyLive, Risk: RiskWeakensSecurity, CheckItem: hostnameCheck,
		Title:       "Hosts that receive an app's git credentials for submodules",
		Description: "A submodule on the parent repository's host is fetched with the app's git credentials; one on any other host is fetched without them. List extra hosts here to send the credentials there too. Empty by default.",
	})
	Register(Definition{
		Key: KeyGitLFSHosts, Category: CategorySecurity, Kind: KindStringList, Default: "",
		Env: "APP_GIT_LFS_HOSTS", Apply: ApplyLive, Risk: RiskWeakensSecurity, CheckItem: hostnameCheck,
		Title:       "Extra hosts LFS objects may download from",
		Description: "LFS objects download only from the repository's own host. List the storage hosts your git server redirects LFS downloads to (for example a CDN host). The app's credentials are never sent to them. Empty by default.",
	})
	Register(Definition{
		Key: KeyGitSubmodulesAllowedCIDRs, Category: CategorySecurity, Kind: KindStringList, Default: "",
		Env: "APP_GIT_SUBMODULES_ALLOWED_CIDRS", Apply: ApplyLive, Risk: RiskWeakensSecurity, CheckItem: cidrOrIPCheck,
		Title:       "Internal networks submodule and LFS hosts may be on",
		Description: "Submodule and LFS fetches refuse loopback, private, link-local, CGNAT, multicast and unspecified addresses, checked at connect time. List the IPs or CIDR ranges of a self-hosted git server here to allow them. Empty by default.",
	})
}

// GitSubmodulesTimeout returns the saved fetch timeout forced into its range.
func (s *Service) GitSubmodulesTimeout(ctx context.Context) time.Duration {
	d := s.Duration(ctx, KeyGitSubmodulesTimeout)
	if d <= 0 {
		return gitSubmodulesTimeoutDef
	}
	return min(max(d, gitSubmodulesTimeoutFloor), gitSubmodulesTimeoutCeil)
}
