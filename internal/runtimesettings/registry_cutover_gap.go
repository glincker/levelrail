package runtimesettings

import (
	"context"
	"time"
)

// Keys for the measured cutover gap on the deploy timeline.
const (
	KeyDeployCutoverGapWindow     = "deploy.cutover_gap_window"
	KeyDeployCutoverGapQueuedOver = "deploy.cutover_gap_queued_over"
)

func init() {
	Register(Definition{
		Key: KeyDeployCutoverGapWindow, Category: CategoryDeploys, Kind: KindDuration, Default: "15s",
		Apply: ApplyLive, CheckValue: durationRangeCheck(time.Second, 10*time.Minute),
		Title:       "Cutover gap: time around cutover measured",
		Description: "How long before and after traffic moves to the new release the deploy timeline counts failed and queued edge requests, from 1s to 10m.",
	})
	Register(Definition{
		Key: KeyDeployCutoverGapQueuedOver, Category: CategoryDeploys, Kind: KindDuration, Default: "1s",
		Apply: ApplyLive, CheckValue: durationRangeCheck(100*time.Millisecond, time.Minute),
		Title:       "Cutover gap: request time that counts as queued",
		Description: "A request near cutover that took at least this long counts as queued in the measured cutover gap, from 100ms to 1m.",
	})
}

// CutoverGapSettings are the live bounds of the cutover gap measurement.
type CutoverGapSettings struct {
	Window     time.Duration
	QueuedOver time.Duration
}

// CutoverGap returns the live cutover gap settings; a nil service serves defaults.
func (s *Service) CutoverGap(ctx context.Context) CutoverGapSettings {
	return CutoverGapSettings{
		Window:     s.Duration(ctx, KeyDeployCutoverGapWindow),
		QueuedOver: s.Duration(ctx, KeyDeployCutoverGapQueuedOver),
	}
}
