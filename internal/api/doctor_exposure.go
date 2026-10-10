package api

import (
	"context"
	"fmt"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/exposure"
)

const (
	doctorExposureCode = "exposure"
	doctorExposureDocs = "/exposure-audit"
)

// doctorCheckExposure reports published container ports the internet can
// reach. Read-only and bounded; the outside probe stays an explicit action.
func (rt *Router) doctorCheckExposure(ctx context.Context) []doctorCheckResource {
	if rt.exposure == nil || rt.apps == nil || rt.databases == nil {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, doctorPingTimeout*2)
	defer cancel()
	rep, err := rt.exposureReport(cctx, "", false)
	const name = "Published ports"
	if err != nil {
		return []doctorCheckResource{{Code: doctorExposureCode, Name: name, Status: doctorStatusUnknown, Message: "could not audit published container ports: " + err.Error()}}
	}
	var out []doctorCheckResource
	attention, unreadable := 0, false
	for _, n := range rep.Nodes {
		if n.Status != exposureNodeOK {
			out = append(out, doctorCheckResource{
				Code: doctorExposureCode + "_" + nodeKey(n) + "_unreachable", Name: name + " (" + n.NodeName + ")", Status: doctorStatusUnknown,
				Message: "could not list containers on this node, so its published ports are unaudited: " + n.Error,
			})
			continue
		}
		for _, f := range n.Findings {
			if !f.NeedsAttention() {
				continue
			}
			if f.Class == exposure.ClassUnknown {
				unreadable = true
			}
			if f.Severity == exposure.SeverityInfo {
				continue
			}
			attention++
			out = append(out, doctorExposureFinding(n, f))
		}
	}
	return append([]doctorCheckResource{doctorExposureSummary(attention, unreadable)}, out...)
}

func nodeKey(n exposureNodeResource) string {
	if n.Local {
		return localNodeLabel
	}
	return n.NodeName
}

func doctorExposureSummary(attention int, unreadable bool) doctorCheckResource {
	const name = "Published ports"
	switch {
	case attention > 0:
		return doctorCheckResource{
			Code: doctorExposureCode, Name: name, Status: doctorStatusWarn,
			Message:  fmt.Sprintf("%d published container port(s) listen on a public address and are not proven restricted. Docker bypasses ufw for published ports", attention),
			Fix:      "levelrail-cli firewall exposure",
			DocsPath: doctorExposureDocs,
		}
	case unreadable:
		return doctorCheckResource{Code: doctorExposureCode, Name: name, Status: doctorStatusUnknown, Message: "published ports found, but the host firewall could not be read to tell whether they are restricted", DocsPath: doctorExposureDocs}
	}
	return doctorCheckResource{Code: doctorExposureCode, Name: name, Status: doctorStatusOK, Message: "nothing of yours is published to the internet without a restriction"}
}

func doctorExposureFinding(n exposureNodeResource, f exposureFindingResource) doctorCheckResource {
	status := doctorStatusWarn
	fix := fmt.Sprintf("levelrail-cli firewall restrict --port %d --protocol %s --allow <your server address>/32 --dry-run", f.HostPort, f.Protocol)
	if !f.CanRestrict || !n.Local {
		fix = ""
	}
	return doctorCheckResource{
		Code:     fmt.Sprintf("%s_%s_%s_%d", doctorExposureCode, nodeKey(n), f.Protocol, f.HostPort),
		Name:     fmt.Sprintf("Port %s/%s (%s) on %s", strconv.Itoa(f.HostPort), f.Protocol, f.Container, n.NodeName),
		Status:   status,
		Message:  fmt.Sprintf("[%s] %s", f.Severity, f.Explanation),
		Fix:      fix,
		DocsPath: doctorExposureDocs,
	}
}
