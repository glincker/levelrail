package golive

import (
	"fmt"
	"net/url"
	"time"

	"github.com/GLINCKER/levelrail/internal/domaindoctor"
)

func noDomainStep(in Input, id, title string) Step {
	return Step{ID: id, Title: title, Status: StatusPending, Evidence: []Evidence{},
		Summary: "Add the domain you want to use first.",
		Primary: &Action{Kind: ActionLink, Label: "Add a domain", Href: "/apps/" + url.PathEscape(in.App) + "/domains"}}
}

func recordText(r domaindoctor.Record) string {
	return fmt.Sprintf("%s %s %s", r.Name, r.Type, r.Value)
}

func dnsStep(in Input) Step {
	const title = "DNS points the domain at this server"
	if in.Domain == "" {
		return noDomainStep(in, StepDNS, title)
	}
	s := Step{ID: StepDNS, Title: title, Evidence: []Evidence{}}
	rep := in.Report
	if rep == nil {
		s.Status, s.Summary, s.Primary = StatusUnknown, "DNS could not be checked just now.", recheck()
		return s
	}
	s.Records = rep.Records
	for _, r := range rep.Records {
		s.Evidence = append(s.Evidence, Evidence{Label: "Record to create", Value: recordText(r), Copy: true})
	}
	var bad *domaindoctor.Check
	for _, id := range []string{domaindoctor.IDDNSResolves, domaindoctor.IDDNSPointsHere, domaindoctor.IDDNSApexCNAME, domaindoctor.IDDNSAAAA, domaindoctor.IDDNSProxied} {
		if c, ok := checkByID(rep, id); ok && (c.State == domaindoctor.StateFail || c.State == domaindoctor.StateWarn) && (bad == nil || c.State == domaindoctor.StateFail && bad.State != domaindoctor.StateFail) {
			cp := c
			bad = &cp
		}
	}
	prop := rep.Propagation
	if prop != nil && prop.Detail != "" {
		s.Evidence = append(s.Evidence, ev("Propagation", prop.Detail))
	}
	switch {
	case bad == nil:
		s.Status, s.Summary = StatusOK, "The domain resolves to this server."
	case bad.State == domaindoctor.StateWarn:
		s.Status, s.Summary = StatusWarn, bad.Title+orSuffix(bad.Detail)
		s.Primary = dnsPrimary(in, bad)
	case prop != nil && prop.State == domaindoctor.PropagationPropagating:
		s.Status, s.Summary = StatusPending, "The record exists but has not reached every resolver yet. "+prop.Detail
		s.Primary = recheck()
		s.NextCheckAt = nextCheck(in, prop)
	default:
		s.Status, s.Summary = StatusFail, bad.Title+orSuffix(bad.Detail)
		s.Primary = dnsPrimary(in, bad)
		if prop != nil && prop.State == domaindoctor.PropagationWaiting {
			s.NextCheckAt = nextCheck(in, prop)
		}
	}
	return s
}

func orSuffix(d string) string {
	if d == "" {
		return ""
	}
	return ": " + d
}

func nextCheck(in Input, p *domaindoctor.Propagation) *time.Time {
	var t time.Time
	switch {
	case !p.NextCheckAt.IsZero():
		t = p.NextCheckAt
	case p.NextCheckSeconds > 0:
		t = in.Now.Add(time.Duration(p.NextCheckSeconds) * time.Second)
	default:
		return nil
	}
	return &t
}

func dnsPrimary(in Input, c *domaindoctor.Check) *Action {
	if c.Fix != nil && c.Fix.Action != nil && c.Fix.Action.Kind == domaindoctor.ActionRequest && c.Fix.Action.API != "" {
		a := &Action{Kind: ActionRequest, Label: c.Fix.Action.Label, API: c.Fix.Action.API}
		if len(c.Fix.Action.Body) > 0 {
			a.Body = map[string]any{}
			for k, v := range c.Fix.Action.Body {
				a.Body[k] = v
			}
		}
		return a
	}
	if in.Report != nil && len(in.Report.Records) > 0 {
		return &Action{Kind: ActionCopy, Label: "Copy the record", Value: recordText(in.Report.Records[0])}
	}
	return recheck()
}
