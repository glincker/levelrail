package runtimesettings

import "time"

// KeyShellSectionTimeout bounds each section of the shell bootstrap aggregate.
const KeyShellSectionTimeout = "shell.section_timeout"

func init() {
	Register(Definition{
		Key: KeyShellSectionTimeout, Category: CategoryNetworking, Kind: KindDuration, Default: "2s",
		Env: "APP_SHELL_SECTION_TIMEOUT", Apply: ApplyLive, CheckValue: durationRangeCheck(200*time.Millisecond, 30*time.Second),
		Title:       "Shell bootstrap section timeout",
		Description: "How long one section of GET /api/v1/shell may take, from 200ms to 30s. A slower section is left out and the dashboard fetches it on its own. The next request uses a changed value.",
	})
}
