package runtimesettings

import (
	"context"
	"time"
)

// KeyFirewallConfirmWindow is how long a dashboard or CLI port restriction waits for confirmation.
const KeyFirewallConfirmWindow = "firewall.confirm_window"

const (
	minFirewallConfirmWindow = 10 * time.Second
	maxFirewallConfirmWindow = 10 * time.Minute
)

func init() {
	Register(Definition{
		Key: KeyFirewallConfirmWindow, Category: CategorySecurity, Kind: KindDuration, Default: "60s",
		Env: "APP_FIREWALL_CONFIRM_WINDOW", Apply: ApplyLive, CheckValue: durationRangeCheck(minFirewallConfirmWindow, maxFirewallConfirmWindow),
		Title:       "Firewall: confirm window",
		Description: "How long a port restriction applied from the dashboard or CLI stays before it is rolled back unless someone confirms it, from 10s to 10m. The next restriction uses a changed value.",
	})
}

// FirewallConfirmWindow reads the confirm window, forced into its safe range.
func (s *Service) FirewallConfirmWindow(ctx context.Context) time.Duration {
	return clampDuration(s.Duration(ctx, KeyFirewallConfirmWindow), minFirewallConfirmWindow, maxFirewallConfirmWindow)
}
