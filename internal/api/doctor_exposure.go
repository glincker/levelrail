package api

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
	var tally exposureTally
	for _, n := range rep.Nodes {
		if n.Status != exposureNodeOK {
			out = append(out, doctorCheckResource{
				Code: doctorExposureCode + "_" + nodeKey(n) + "_unreachable", Name: name + " (" + n.NodeName + ")", Status: doctorStatusUnknown,
				Message: "could not list containers on this node, so its published ports are unaudited: " + n.Error,
			})
			continue
		}
		out = append(out, doctorExposureContainerRows(n, &tally)...)
	}
	out = append(out, doctorExposureDrift(rep.Drift)...)
	return append([]doctorCheckResource{doctorExposureSummary(tally)}, out...)
}

// doctorExposureDrift turns drift findings into rows. Hand-made rules are
// informational and unmanaged public ports already have a row each, so both stay out.
func doctorExposureDrift(drift []exposure.DriftFinding) []doctorCheckResource {
	var out []doctorCheckResource
	for _, d := range drift {
		if d.Severity == exposure.SeverityInfo || d.Kind == exposure.DriftUnmanagedPublicPort {
			continue
		}
		name := fmt.Sprintf("Exposure drift %d/%s", d.Port, d.Protocol)
		if len(d.Ports) > 1 {
			name = "Exposure drift " + strings.Join(d.Ports, ", ")
		}
		out = append(out, doctorCheckResource{
			Code:     fmt.Sprintf("%s_drift_%s_%s_%d", doctorExposureCode, d.Kind, d.Protocol, d.Port),
			Name:     name,
			Status:   doctorStatusWarn,
			Message:  d.Message,
			Fix:      d.Fix,
			DocsPath: doctorExposureDocs,
		})
	}
	return out
}

func nodeKey(n exposureNodeResource) string {
	if n.Local {
		return localNodeLabel
	}
	return n.NodeName
}

// exposureTally counts what the per-container rows list, so the summary
// can never disagree with them.
type exposureTally struct {
	ports, warnPorts, containers, foreign int
	unreadable                            bool
}

func doctorExposureSummary(t exposureTally) doctorCheckResource {
	const name = "Published ports"
	switch {
	case t.warnPorts > 0:
		return doctorCheckResource{
			Code: doctorExposureCode, Name: name, Status: doctorStatusWarn,
			Message:  fmt.Sprintf("%d published container port(s) in %d container(s) listen on a public address and are not proven restricted (%d container(s) managed by another tool). Docker bypasses ufw for published ports", t.ports, t.containers, t.foreign),
			Fix:      "levelrail-cli firewall exposure",
			DocsPath: doctorExposureDocs,
		}
	case t.unreadable:
		return doctorCheckResource{Code: doctorExposureCode, Name: name, Status: doctorStatusUnknown, Message: "published ports found, but the host firewall could not be read to tell whether they are restricted", DocsPath: doctorExposureDocs}
	case t.ports > 0:
		return doctorCheckResource{
			Code: doctorExposureCode, Name: name, Status: doctorStatusOK,
			Message:  fmt.Sprintf("%d published container port(s) in %d container(s) are public by design or low risk (%d container(s) managed by another tool). Review them if you did not expect them", t.ports, t.containers, t.foreign),
			DocsPath: doctorExposureDocs,
		}
	}
	return doctorCheckResource{Code: doctorExposureCode, Name: name, Status: doctorStatusOK, Message: "nothing of yours is published to the internet without a restriction"}
}

type exposureGroup struct {
	findings []exposureFindingResource
	ports    []string
	seen     map[string]bool
	warn     bool
	restrict bool
	foreign  bool
	worst    exposure.Severity
}

// doctorExposureContainerRows folds a node's findings into one row per
// container. Info findings stay out; only medium and high make a warning.
func doctorExposureContainerRows(n exposureNodeResource, t *exposureTally) []doctorCheckResource {
	groups := map[string]*exposureGroup{}
	var order []string
	for _, f := range n.Findings {
		if !f.NeedsAttention() {
			continue
		}
		if f.Class == exposure.ClassUnknown {
			t.unreadable = true
		}
		if f.Severity == exposure.SeverityInfo {
			continue
		}
		g := groups[f.Container]
		if g == nil {
			g = &exposureGroup{seen: map[string]bool{}, restrict: true, foreign: f.Owner.Kind == exposure.OwnerUnmanaged, worst: exposure.SeverityLow}
			groups[f.Container] = g
			order = append(order, f.Container)
		}
		key := fmt.Sprintf("%d/%s", f.HostPort, f.Protocol)
		if g.seen[key] {
			continue
		}
		g.seen[key] = true
		g.ports = append(g.ports, key)
		g.findings = append(g.findings, f)
		g.restrict = g.restrict && f.CanRestrict
		if f.Severity == exposure.SeverityMedium || f.Severity == exposure.SeverityHigh {
			g.warn = true
			if g.worst != exposure.SeverityHigh {
				g.worst = f.Severity
			}
		}
	}
	sort.Strings(order)
	var out []doctorCheckResource
	for _, c := range order {
		g := groups[c]
		status := doctorStatusOK
		if g.warn {
			status = doctorStatusWarn
			t.warnPorts += len(g.ports)
		}
		t.ports += len(g.ports)
		t.containers++
		owner := "managed by this control plane"
		action := "Restrict it with a DOCKER-USER rule, or rebind it to localhost if only this host needs it."
		if g.foreign {
			t.foreign++
			owner = "managed by another tool"
			action = "If it should be public, route it through the ingress; otherwise restrict it with a DOCKER-USER rule."
		}
		fix := "levelrail-cli firewall exposure"
		if len(g.findings) == 1 && g.restrict && n.Local {
			f := g.findings[0]
			fix = fmt.Sprintf("levelrail-cli firewall restrict --port %d --protocol %s --allow <your server address>/32 --dry-run", f.HostPort, f.Protocol)
		}
		out = append(out, doctorCheckResource{
			Code:     fmt.Sprintf("%s_%s_%s", doctorExposureCode, nodeKey(n), c),
			Name:     fmt.Sprintf("Container %s on %s", c, n.NodeName),
			Status:   status,
			Message:  fmt.Sprintf("%s, %s. Public ports: %s. %s", owner, g.worst, strings.Join(g.ports, ", "), action),
			Fix:      fix,
			DocsPath: doctorExposureDocs,
		})
	}
	return out
}
