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
	// calm drops the crashloop and the disk alert so the estate looks healthy.
	calm bool
}

func storyFor(anchor time.Time, calm bool) storyTimes {
	slow := anchor.AddDate(0, 0, -3).Add(3*time.Hour + 7*time.Minute)
	st := storyTimes{
		calm:          calm,
		slowDeploy:    slow,
		slowRecovered: slow.Add(41 * time.Minute),
		crashStart:    anchor.Add(-32 * time.Minute),
		crashEnd:      anchor.Add(-20 * time.Minute),
		diskFired:     anchor.Add(-50 * time.Minute),
	}
	if calm {
		// Zero times make every crash window check false.
		st.crashStart, st.crashEnd = time.Time{}, time.Time{}
	}
	return st
}
