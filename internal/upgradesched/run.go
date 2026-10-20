package upgradesched

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
)

// DefaultInterval is how often Run evaluates schedules.
const DefaultInterval = 30 * time.Second

// Run evaluates the schedule on every interval until ctx is done.
func (s *Scheduler) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return footprint.Every(ctx, "upgradesched", interval, s.log(), s.Tick)
}

// NewScheduleID returns a random schedule id.
func NewScheduleID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "us_" + hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405")))
	}
	return "us_" + hex.EncodeToString(b)
}
