package iac

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func anyList(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

func copyFields(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// desiredDomains folds Domain documents that target the app into its own
// domain list, so the two declaration styles never fight over ownership.
func (p *planner) desiredDomains(r *Resource) []string {
	hosts := stringList(r.Fields["domains"])
	for _, o := range p.res {
		if o.Kind == KindDomain && stringField(o.Fields, "app") == r.Name {
			hosts = append(hosts, o.Name)
		}
	}
	return sortedUnique(hosts)
}

func (p *planner) secretValue(app, name string) (string, bool) {
	if v, ok := p.opts.Secrets[app+"/"+name]; ok {
		return v, true
	}
	v, ok := p.opts.Secrets[name]
	return v, ok
}

func (p *planner) planApp(r *Resource) {
	if p.ledgerGate(r) {
		return
	}
	defer p.ledgerNote(r)
	if p.deniedItem(r, r.Key()) {
		return
	}
	f := copyFields(r.Fields)
	if d := p.desiredDomains(r); len(d) > 0 {
		f["domains"] = anyList(d)
	}
	if !p.checkAppRefs(r, f) {
		return
	}
	deps, ok := p.checkConnRefs(r, f)
	if !ok {
		return
	}
	jobs := desiredJobs(f)
	delete(f, "jobs")
	conns := desiredConns(f)
	live, exists := p.st.apps[r.Name]
	if _, denied := p.st.denied["jobs/"+r.Name]; denied && jobs != nil {
		p.fail(r, "the jobs of app %s cannot be read: %v", r.Name, p.st.denied["jobs/"+r.Name])
		return
	}
	var setSecrets []string
	for _, n := range r.SecretRefs {
		_, supplied := p.secretValue(r.Name, n)
		switch {
		case supplied:
			setSecrets = append(setSecrets, n)
		case !exists || !live.secrets[n]:
			r.Warnings = append(r.Warnings, fmt.Sprintf("secret %s has no value yet; the app cannot use it until one is set", n))
		}
	}
	managed := p.st.managed[r.Name]
	prune := p.opts.Prune && managed
	var liveRef *liveApp
	if exists {
		liveRef = live
	}
	jobChanges, jobKept, jobOps := p.planJobs(r, jobs, liveRef, prune)
	r.Warnings = append(r.Warnings, connWarnings(conns, liveRef)...)
	if !exists {
		fields := append(addAll(f, additiveApp), jobChanges...)
		sort.SliceStable(fields, func(i, j int) bool { return fields[i].Path < fields[j].Path })
		it := p.add(r, ActionCreate, fields, nil, nil)
		it.DependsOn = deps
		it.exec = func(ctx context.Context, x *executor) error {
			if err := x.createApp(ctx, r, f, setSecrets); err != nil {
				return err
			}
			return x.syncJobs(ctx, r.Name, jobOps)
		}
		return
	}
	liveMap := toMap(appFromWire(live.wire, idNames{p.st}))
	for _, k := range []string{"project", "environment"} {
		if _, want := f[k]; !want {
			delete(liveMap, k)
		}
	}
	if want, _ := f["image"].(string); want != "" {
		if have, _ := liveMap["image"].(string); sameDeclaredImage(want, have) {
			liveMap["image"] = want
		}
	}
	changes, kept := diffFields(f, liveMap, additiveApp, prune)
	changes = append(changes, jobChanges...)
	kept = append(kept, jobKept...)
	if u := unsupportedApp(live.wire); len(u) > 0 {
		r.Warnings = append(r.Warnings, "live settings apply does not manage are kept: "+strings.Join(u, ", "))
	}
	for _, n := range setSecrets {
		changes = append(changes, FieldChange{Path: "secrets." + n, Op: OpChange, New: hiddenValue})
	}
	if len(changes) == 0 {
		p.add(r, ActionNoop, nil, kept, nil).DependsOn = deps
		return
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	it := p.add(r, ActionUpdate, changes, kept, nil)
	it.DependsOn = deps
	it.exec = func(ctx context.Context, x *executor) error {
		if err := x.updateApp(ctx, r, f, live, prune, setSecrets); err != nil {
			return err
		}
		return x.syncJobs(ctx, r.Name, jobOps)
	}
}

func (p *planner) checkAppRefs(r *Resource, f map[string]any) bool {
	if proj := stringField(f, "project"); proj != "" && !p.projectKnown(proj) {
		p.fail(r, "project %q does not exist and is not declared in the files", proj)
		return false
	}
	env := stringField(f, "environment")
	if env == "" {
		return true
	}
	proj := stringField(f, "project")
	_, live := p.st.envs[envKey(proj, env)]
	_, declared := p.byKey[resourceKey(KindEnvironment, proj, env)]
	if !live && !declared {
		p.fail(r, "environment %q does not exist in project %q and is not declared in the files", env, proj)
		return false
	}
	return true
}

func (p *planner) planPrune() {
	if p.st.ledger != nil {
		p.planLedgerPrune()
		return
	}
	if p.opts.Source == "" {
		return
	}
	for _, name := range p.st.ManagedApps() {
		if _, kept := p.byKey[resourceKey(KindApp, "", name)]; kept {
			p.pruneLB(name)
			continue
		}
		if p.opts.Project != "" {
			live, ok := p.st.apps[name]
			if !ok || p.st.projectsByID[live.wire.ProjectID].Name != p.opts.Project {
				continue
			}
		}
		r := &Resource{Kind: KindApp, Name: name}
		p.add(r, ActionDelete, nil, nil, func(ctx context.Context, x *executor) error {
			return x.d.Do(ctx, "DELETE", "/api/v1/apps/"+esc(name), nil, nil)
		}).Reason = "carries the managed-by tag for source " + p.opts.Source + " and is absent from the files"
	}
}

func (p *planner) pruneLB(app string) {
	if _, declared := p.byKey[resourceKey(KindLoadBalancer, "", app)]; declared {
		return
	}
	if lb, ok := p.st.lbs[app]; !ok || !lb.Configured {
		return
	}
	r := &Resource{Kind: KindLoadBalancer, Name: app}
	p.add(r, ActionDelete, nil, nil, func(ctx context.Context, x *executor) error {
		return x.d.Do(ctx, "DELETE", "/api/v1/apps/"+esc(app)+"/loadbalancer", nil, nil)
	}).Reason = "app is managed by source " + p.opts.Source + " and the load balancer is absent from the files"
}

// sameDeclaredImage reports whether the live image is the declared one. The
// runtime pins a fresh app to repo:tag@digest, so a digest on the live side
// only counts when the declaration pins one too.
func sameDeclaredImage(declared, live string) bool {
	if !strings.Contains(declared, "@") {
		live, _, _ = strings.Cut(live, "@")
	}
	return docker.SameImageRef(withDefaultTag(declared), withDefaultTag(live))
}

func withDefaultTag(ref string) string {
	if strings.Contains(ref, "@") {
		return ref
	}
	if i := strings.LastIndex(ref, "/"); !strings.Contains(ref[i+1:], ":") {
		return ref + ":latest"
	}
	return ref
}
