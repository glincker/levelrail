package api

import (
	"context"
	"os"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// buildMemorySuggestion sizes a new per-app build cap when the newest run was killed for
// memory. Nil once the app's cap already covers the suggestion, so a click is never offered twice.
func (rt *Router) buildMemorySuggestion(ctx context.Context, app string, newest []store.BuildRun) *build.MemorySuggestion {
	if len(newest) == 0 || newest[0].Status != string(build.RunMemoryLimit) {
		return nil
	}
	current := rt.appBuildLimitsView(ctx, app).Effective.MemoryMB
	peak := newest[0].PeakMemoryMB
	limit := current
	if peak > 0 {
		// A measured peak already includes the cap that was hit; a raised cap must not inflate it.
		limit = 0
	}
	read := rt.doctorHostMemory
	if read == nil {
		read = func() (int64, int64, error) { return telemetry.HostMemoryBytes() }
	}
	hostMB := 0
	if total, _, err := read(); err == nil {
		hostMB = int(total >> 20)
	}
	s := build.SuggestMemory(limit, peak, hostMB, build.MemGuardFromEnv(os.LookupEnv))
	if s.SuggestedMB == 0 || (current > 0 && current >= s.SuggestedMB) {
		return nil
	}
	s.LimitMB = current
	return &s
}
