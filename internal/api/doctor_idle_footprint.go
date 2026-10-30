package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/footprint"
)

func (rt *Router) doctorCheckIdleFootprint(context.Context) doctorCheckResource {
	const code, name = "idle_footprint", "Control plane footprint"
	p := rt.cpHealth.probe.Load()
	if p == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "control plane health is not available"}
	}
	a := rt.idleAssessment(p)
	cur := a.Current
	line := fmt.Sprintf("rss %d MiB, cpu %.1f%%, %d goroutines, %d open fds", cur.RSSBytes>>20, cur.CPUPercent, cur.Goroutines, cur.OpenFDs)
	switch a.Status {
	case footprint.StatusWithin:
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: line + ", within tolerance of the idle baseline"}
	case footprint.StatusOver:
		var over []string
		for _, c := range a.Checks {
			if !c.OK {
				over = append(over, fmt.Sprintf("%s %.1f over limit %.1f", c.Name, c.Current, c.Limit))
			}
		}
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message: line + ", above the idle baseline: " + strings.Join(over, ", "),
			Fix:     rt.cliName() + " system health", DocsPath: "/performance",
		}
	default:
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: line + ". " + a.Detail}
	}
}
