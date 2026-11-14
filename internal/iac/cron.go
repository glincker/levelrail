package iac

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

var cronNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// liveJobs returns the jobs read for an app, nil when none were read.
func (st *State) liveJobs(app string) map[string]wireJob {
	if a, ok := st.apps[app]; ok {
		return a.jobs
	}
	return nil
}

// cronSpecOut drops defaults so an exported document stays short.
func cronSpecOut(app string, f jobFields) map[string]any {
	m := toMap(f)
	m["app"] = app
	if f.Mode == spec.JobModeFresh {
		delete(m, "mode")
	}
	if f.Concurrency == "forbid" {
		delete(m, "concurrency")
	}
	if f.CatchUp == jobCatchUpSkip {
		delete(m, "catchUp")
	}
	return m
}

func buildCron(d *Document, s *issueSink) *Resource {
	var sp struct {
		App      string `yaml:"app"`
		spec.Job `yaml:",inline"`
	}
	decodeSpec(d, &sp, s)
	sp.Name = d.Name
	if !cronNameRe.MatchString(d.Name) || strings.HasPrefix(d.Name, spec.ReservedJobNamePrefix) {
		s.atCode(CodeCronJobInvalid, "metadata.name", "job name %q must be lowercase letters, digits, hyphens or underscores and must not start with %q", d.Name, spec.ReservedJobNamePrefix)
	}
	if len(sp.Command) == 0 {
		s.atCode(CodeCronJobInvalid, "spec.command", "a cron job needs a command")
	}
	for _, part := range sp.Command {
		if strings.TrimSpace(part) == "" {
			s.atCode(CodeCronJobInvalid, "spec.command", "command must not contain empty arguments")
			break
		}
	}
	loc := time.UTC
	if sp.Timezone != "" {
		l, err := time.LoadLocation(sp.Timezone)
		if err != nil {
			s.atCode(CodeCronJobInvalid, "spec.timezone", "timezone %q is not a known IANA time zone", sp.Timezone)
		} else {
			loc = l
		}
	}
	if err := cronexpr.Validate(sp.Schedule, loc, time.Now()); err != nil {
		s.atCode(CodeCronScheduleInvalid, "spec.schedule", "schedule %q is not a valid cron expression: %v", sp.Schedule, err)
	}
	if sp.Mode == spec.JobModeExec && (sp.CPU != 0 || sp.Memory != "") {
		s.atCode(CodeCronJobInvalid, "spec.mode", "cpu and memory limits apply to fresh-container jobs, not mode: exec")
	}
	if sp.Retries < 0 {
		s.atCode(CodeCronJobInvalid, "spec.retries", "retries must not be negative")
	}
	tmp := &issueSink{d: d}
	f := jobsFromSpec([]spec.Job{sp.Job}, tmp)[d.Name]
	for _, is := range tmp.issues {
		is.Path = "spec"
		s.issues = append(s.issues, is)
	}
	return &Resource{Kind: KindCronJob, Name: d.Name, Scope: sp.App, Doc: d, Fields: toMap(f)}
}

func (p *planner) planCron(r *Resource) {
	if p.ledgerGate(r) {
		return
	}
	defer p.ledgerNote(r)
	if p.deniedItem(r, "jobs/"+r.Scope) {
		return
	}
	if !p.appKnown(r.Scope) {
		p.fail(r, "app %q does not exist and is not declared in the files", r.Scope)
		return
	}
	if app, ok := p.byKey[resourceKey(KindApp, "", r.Scope)]; ok {
		if _, dup := desiredJobs(app.Fields)[r.Name]; dup {
			p.failCode(r, CodeOwnershipConflict, "job %s is declared both here and in the jobs of app %s; declare it in one place", r.Name, r.Scope)
			return
		}
	}
	live, exists := p.st.liveJobs(r.Scope)[r.Name]
	if exists && live.Source == jobSourceSpec {
		p.failCode(r, CodeOwnershipConflict, "job %s is declared in the app's own app.yaml; declare it in one place", r.Name)
		return
	}
	var want jobFields
	if err := remarshal(r.Fields, &want); err != nil {
		p.fail(r, "job %s could not be read: %v", r.Name, err)
		return
	}
	base := "/api/v1/apps/" + esc(r.Scope) + "/jobs"
	if !exists {
		p.add(r, ActionCreate, addAll(r.Fields, nil), nil, func(ctx context.Context, x *executor) error {
			return x.do(ctx, http.MethodPost, base, jobBody(r.Name, want), nil)
		})
		return
	}
	changes, _ := diffFields(r.Fields, toMap(jobFromWire(live)), nil, false)
	if len(changes) == 0 {
		p.add(r, ActionNoop, nil, nil, nil)
		return
	}
	for i := range changes {
		if changes[i].Path == "command" && envredact.SensitiveValue("COMMAND", changes[i].Old) {
			changes[i].Old = hiddenValue
		}
	}
	p.add(r, ActionUpdate, changes, nil, func(ctx context.Context, x *executor) error {
		return x.do(ctx, http.MethodPut, base+"/"+esc(live.ID), jobBody(r.Name, want), nil)
	})
}
