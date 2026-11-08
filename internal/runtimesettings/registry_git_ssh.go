package runtimesettings

import (
	"fmt"
	"net/netip"
	"strings"
)

// KeyGitSSHScanAllowInternal lists internal networks a host key scan may dial.
const KeyGitSSHScanAllowInternal = "git.ssh_scan_allow_internal"

func cidrOrIPCheck(item string) error {
	item = strings.TrimSpace(item)
	if _, err := netip.ParsePrefix(item); err == nil {
		return nil
	}
	if _, err := netip.ParseAddr(item); err == nil {
		return nil
	}
	return fmt.Errorf("%q must be an IP address or a CIDR range", item)
}

func init() {
	Register(Definition{
		Key: KeyGitSSHScanAllowInternal, Category: CategorySecurity, Kind: KindStringList, Default: "",
		Env: "APP_GIT_SSH_SCAN_ALLOW_INTERNAL", Apply: ApplyLive, Risk: RiskWeakensSecurity, CheckItem: cidrOrIPCheck,
		Title:       "Internal networks a git SSH host key scan may reach",
		Description: "Trust-on-first-use host key scans refuse loopback, private and link-local addresses. List the IPs or CIDR ranges of a self-hosted git server here to allow them. Cloud metadata addresses are never allowed. Empty by default.",
	})
}
