package api

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// certWeeklyIssueWarn reads the warning threshold at call time; without a
// settings service the registered default applies.
func (rt *Router) certWeeklyIssueWarn(ctx context.Context) int {
	svc := rt.runtimeSettings
	if svc == nil {
		svc = runtimesettings.New(nil, func(string) (string, bool) { return "", false })
	}
	return svc.Int(ctx, runtimesettings.KeyCertificatesWeeklyIssueWarn)
}
