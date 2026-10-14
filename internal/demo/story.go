package demo

import "time"

// storyTimes places the demo's narrative moments relative to the anchor, so
// metrics, logs, deploys and alerts all agree and a reseed always looks recent.
type storyTimes struct {
	// slowDeploy is when the regressing deploy of incidentApp cut over.
	slowDeploy time.Time
	// slowRecovered is when the automatic rollback finished.
	slowRecovered time.Time
	// crashStart and crashEnd bound the crashloop of crashloopApp.
	crashStart, crashEnd time.Time
	// diskFired is when the worker disk alert started firing.
	diskFired time.Time
}

func storyFor(anchor time.Time) storyTimes {
	slow := anchor.AddDate(0, 0, -3).Add(3*time.Hour + 7*time.Minute)
	return storyTimes{
		slowDeploy:    slow,
		slowRecovered: slow.Add(41 * time.Minute),
		crashStart:    anchor.Add(-32 * time.Minute),
		crashEnd:      anchor.Add(-20 * time.Minute),
		diskFired:     anchor.Add(-50 * time.Minute),
	}
}
