package runtimesettings

import (
	"context"
	"time"
)

// KeyPagesServingWait bounds how long a Pages promote or rollback waits for the new files to be served.
const KeyPagesServingWait = "pages.serving_wait"

func init() {
	Register(Definition{
		Key: KeyPagesServingWait, Category: CategoryDeploys, Kind: KindDuration, Default: "8s",
		Env: "APP_PAGES_SERVING_WAIT", Apply: ApplyLive, CheckValue: durationRangeCheck(0, 2*time.Minute),
		Title:       "Pages: serving wait after promote",
		Description: "How long a Pages promote or rollback waits for the ingress to serve the new files before it answers. If the wait ends first the call still succeeds and reports the route as pending. 0 does not wait.",
	})
}

// PagesServingWait returns the saved wait for a promoted deploy to go live.
func (s *Service) PagesServingWait(ctx context.Context) time.Duration {
	return s.Duration(ctx, KeyPagesServingWait)
}
