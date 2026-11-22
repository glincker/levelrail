package runtimesettings

import "time"

// Keys for how the CLI waits for a new domain to go live.
const (
	KeyDomainsWaitTimeout = "domains.wait_timeout"
	KeyDomainsWaitPoll    = "domains.wait_poll"
)

const (
	domainsWaitPollFloor = time.Second
	domainsWaitPollCeil  = time.Minute
	domainsWaitMaxCeil   = 24 * time.Hour
)

func init() {
	Register(Definition{
		Key: KeyDomainsWaitTimeout, Category: CategoryNetworking, Kind: KindDuration, Default: "10m", Env: "APP_DOMAINS_WAIT_TIMEOUT", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(domainsWaitPollFloor, domainsWaitMaxCeil),
		Title:       "Domains: wait timeout",
		Description: "How long the CLI waits for a new domain to resolve and get its certificate when run with --wait and no --timeout, from 1s to 24h.",
	})
	Register(Definition{
		Key: KeyDomainsWaitPoll, Category: CategoryNetworking, Kind: KindDuration, Default: "3s", Env: "APP_DOMAINS_WAIT_POLL", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(domainsWaitPollFloor, domainsWaitPollCeil),
		Title:       "Domains: wait poll interval",
		Description: "How often the CLI asks for the go-live state while waiting, from 1s to 1m. The --poll flag overrides it.",
	})
}
