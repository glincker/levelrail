package runtimesettings

import "context"

// KeyPagesAnalyticsRetentionDays caps the longest Pages analytics range served.
const KeyPagesAnalyticsRetentionDays = "pages.analytics_retention_days"

func init() {
	Register(Definition{
		Key: KeyPagesAnalyticsRetentionDays, Category: CategoryObservability, Kind: KindInt, Default: "30",
		Env: "APP_PAGES_ANALYTICS_RETENTION_DAYS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 90),
		Title:       "Pages: analytics retention",
		Description: "Longest range in days the Pages analytics endpoint serves, from 1 to 90. Analytics are derived from the edge access log; no client address, cookie or query string is stored. Applies on the next read.",
	})
}

// PagesAnalyticsRetentionDays returns the saved retention in days.
func (s *Service) PagesAnalyticsRetentionDays(ctx context.Context) int {
	return s.Int(ctx, KeyPagesAnalyticsRetentionDays)
}
