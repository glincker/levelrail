package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/schedulescan"
)

// SetScheduleScan wires the store the schedule never runs check reads.
func (rt *Router) SetScheduleScan(src schedulescan.Source) { rt.scheduleScan = src }

func (rt *Router) doctorCheckScheduleNeverRuns(ctx context.Context) doctorCheckResource {
	const code, name = "schedule_never_runs", "Saved schedules can run"
	if rt.scheduleScan == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "schedules could not be read"}
	}
	found, err := schedulescan.Scan(ctx, rt.scheduleScan, time.Now())
	if len(found) == 0 {
		if err != nil {
			return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: err.Error()}
		}
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "every saved schedule has a next run"}
	}
	lines := make([]string, 0, len(found))
	for _, f := range found {
		lines = append(lines, fmt.Sprintf("%s %s (%s): %s", f.Kind, f.ID, f.App, f.Expr))
	}
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message:  fmt.Sprintf("%d saved schedule(s) never run", len(found)),
		Evidence: strings.Join(lines, "; "),
		Fix:      "edit or delete the schedule",
		DocsPath: "/backups",
	}
}
